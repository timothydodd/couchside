package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/keylock"
	"github.com/timothydodd/couchside/internal/parse"
	"github.com/timothydodd/couchside/internal/probe"
	"github.com/timothydodd/couchside/internal/transcode"
)

// subtitleTrack is one choice in the player's Subtitles menu.
type subtitleTrack struct {
	Key      string `json:"key"` // "s2" = embedded stream 2, "x0" = sidecar file 0
	Language string `json:"language"`
	Title    string `json:"title"`
	Codec    string `json:"codec"`
	Text     bool   `json:"text"` // false = picture subs, must be burned in (stream index in BurnIndex)
	Forced   bool   `json:"forced"`
	Default  bool   `json:"default"`
	External bool   `json:"external"`
	Index    int    `json:"index"` // embedded stream index (0:s:N), -1 for sidecars
}

var sidecarExts = map[string]bool{".srt": true, ".vtt": true, ".ass": true, ".ssa": true}

// sidecars finds subtitle files next to a video: "Movie.srt", "Movie.en.srt",
// "Movie.en.forced.srt". A file that fits a longer video name in the same
// folder is that video's: "Alien.Resurrection.srt" isn't Alien.mkv's.
func sidecars(videoPath string) []string {
	dir := filepath.Dir(videoPath)
	stem := videoStem(videoPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var longer []string
	for _, e := range entries {
		if n := e.Name(); !e.IsDir() && parse.IsVideo(n) {
			if o := videoStem(n); len(o) > len(stem) && strings.HasPrefix(o, stem+".") {
				longer = append(longer, o)
			}
		}
	}
	var out []string
next:
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !sidecarExts[strings.ToLower(filepath.Ext(n))] {
			continue
		}
		if _, ok := sidecarTags(stem, n); !ok {
			continue
		}
		for _, o := range longer {
			if strings.HasPrefix(n, o+".") {
				continue next
			}
		}
		out = append(out, filepath.Join(dir, n))
	}
	return out
}

func videoStem(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

var (
	reSubLang  = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,4})?$`)
	subFlagTag = map[string]bool{"forced": true, "sdh": true, "cc": true, "default": true, "hi": true}
)

// sidecarTags reads the tags between a video's stem and a subtitle file's
// extension ("Movie.en.forced.srt" → [en forced]). ok is false unless the
// name is the stem plus at most two tags, each a language code ("en", "eng",
// "pt-BR") or one of forced, sdh, cc, default and hi.
func sidecarTags(stem, name string) (tags []string, ok bool) {
	if !strings.HasPrefix(name, stem+".") {
		return nil, false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(name, stem), filepath.Ext(name))
	if mid == "" {
		return nil, true
	}
	tags = strings.Split(strings.TrimPrefix(mid, "."), ".")
	if len(tags) > 2 {
		return nil, false
	}
	for _, t := range tags {
		if l := strings.ToLower(t); !subFlagTag[l] && !reSubLang.MatchString(l) {
			return nil, false
		}
	}
	return tags, true
}

func (s *Server) fileStreams(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	f, err := s.db.File(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	st, err := probe.ListStreams(r.Context(), s.cfg.FFprobe, f.Path)
	if err != nil {
		slog.Warn("list streams", "file", f.Path, "err", err)
		writeErr(w, httpError{http.StatusBadGateway, "couldn't read this file's audio and subtitle tracks"})
		return
	}
	subs := []subtitleTrack{}
	for _, sub := range st.Subtitles {
		subs = append(subs, subtitleTrack{Key: fmt.Sprintf("s%d", sub.Index), Language: sub.Language, Title: sub.Title,
			Codec: sub.Codec, Text: sub.Text, Forced: sub.Forced, Default: sub.Default, Index: sub.Index})
	}
	stem := videoStem(f.Path)
	for i, p := range sidecars(f.Path) {
		// "Movie.en.forced.srt" → language "en", forced
		tags, _ := sidecarTags(stem, filepath.Base(p))
		t := subtitleTrack{Key: fmt.Sprintf("x%d", i), Codec: strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), "."),
			Text: true, External: true, Index: -1}
		for _, tag := range tags {
			switch l := strings.ToLower(tag); l {
			case "forced":
				t.Forced = true
			case "default":
				t.Default = true
			case "sdh", "cc", "hi":
				t.Title = strings.ToUpper(l)
			default:
				t.Language = l
			}
		}
		subs = append(subs, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"audio": st.Audio, "subtitles": subs})
}

// SubtitleChunk is the window, in seconds, of one embedded-subtitle chunk.
const SubtitleChunk = 90

var (
	// "s2.vtt" = whole embedded track, "s2.c5.vtt" = chunk 5 of it, "x0.vtt" = sidecar file
	reSubKey = regexp.MustCompile(`^([sx])(\d+)(?:\.c(\d+))?\.vtt$`)
	subLocks keylock.Map[string] // by cache path, so one conversion runs per track
	// subSlots caps conversions running at once; others wait while their
	// request lasts.
	subSlots = make(chan struct{}, 3)
)

const subsKeep = 30 * 24 * time.Hour // converted subtitles unused this long are pruned

// subtitleVTT converts a text subtitle track (embedded or sidecar) to WebVTT,
// cached. Timestamps are file time, which is also the player's timeline for
// both direct play and HLS (sessions use -copyts -start_at_zero).
func (s *Server) subtitleVTT(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	m := reSubKey.FindStringSubmatch(chi.URLParam(r, "key"))
	if m == nil {
		http.NotFound(w, r)
		return
	}
	n, _ := strconv.Atoi(m[2])
	f, err := s.db.File(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	var input []string
	chunkTag := ""
	switch {
	case m[1] == "s" && m[3] != "":
		// One chunk of an embedded track. Reading a whole 30 GB remux over the
		// network takes minutes, so the player loads subtitles in windows
		// around the playhead. Start 10s early so cues spanning the boundary
		// aren't lost; copyts keeps cue times relative to the file start.
		// The end is an absolute output time (-to): with -copyts, an input -t
		// is ignored and ffmpeg would read to the end of the file.
		k, _ := strconv.Atoi(m[3])
		if f.DurationSec != nil && float64(k*SubtitleChunk) >= *f.DurationSec {
			http.NotFound(w, r) // past the end: no chunk, and no cache file
			return
		}
		from := max(0, k*SubtitleChunk-10)
		input = []string{"-ss", strconv.Itoa(from), "-copyts", "-start_at_zero",
			"-i", f.Path, "-map", fmt.Sprintf("0:s:%d", n), "-to", strconv.Itoa((k + 1) * SubtitleChunk)}
		chunkTag = fmt.Sprintf("-c%d", k)
	case m[1] == "s":
		input = []string{"-i", f.Path, "-map", fmt.Sprintf("0:s:%d", n)}
	case m[1] == "x":
		sc := sidecars(f.Path)
		if n >= len(sc) {
			http.NotFound(w, r)
			return
		}
		input = []string{"-i", sc[n]}
		// Keyed by the sidecar itself, so an edited, added or removed one
		// never serves another's cues.
		st, err := os.Stat(sc[n])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", filepath.Base(sc[n]), st.Size(), st.ModTime().UnixNano())))
		chunkTag = "-" + hex.EncodeToString(sum[:6])
	}
	cache := filepath.Join(s.cfg.CacheDir, "subs", fmt.Sprintf("%d-%s%d-%d%s.vtt", id, m[1], n, f.Mtime, chunkTag))
	defer subLocks.Lock(cache)()
	if info, err := os.Stat(cache); err == nil {
		if time.Since(info.ModTime()) > 24*time.Hour {
			now := time.Now()
			_ = os.Chtimes(cache, now, now) // still in use: keep it past the prune
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
			writeErr(w, err)
			return
		}
		select {
		case subSlots <- struct{}{}:
			defer func() { <-subSlots }()
		case <-r.Context().Done():
			return
		}
		// Embedded tracks mean reading through the whole file; give it time.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Minute)
		defer cancel()
		tmp := cache + ".tmp"
		args := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}, input...)
		args = append(args, "-c:s", "webvtt", "-f", "webvtt", tmp)
		if out, err := exec.CommandContext(ctx, s.cfg.FFmpeg, args...).CombinedOutput(); err != nil {
			os.Remove(tmp)
			slog.Warn("subtitle conversion failed", "file", id, "track", chunk(m), "err", err, "ffmpeg", transcode.Tail(string(out), 400))
			msg := "couldn't convert subtitles; see the server log"
			if ctx.Err() != nil {
				msg = "couldn't convert subtitles: timed out reading the file"
			}
			writeErr(w, httpError{http.StatusUnprocessableEntity, msg})
			return
		}
		if err := os.Rename(tmp, cache); err != nil {
			writeErr(w, err)
			return
		}
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, cache)
}

func chunk(m []string) string { return m[1] + m[2] + m[3] }

// pruneSubs deletes converted subtitles nobody has loaded in subsKeep.
func (s *Server) pruneSubs() {
	cutoff := time.Now().Add(-subsKeep)
	removed := 0
	entries, _ := os.ReadDir(filepath.Join(s.cfg.CacheDir, "subs"))
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() && info.ModTime().Before(cutoff) {
			if os.Remove(filepath.Join(s.cfg.CacheDir, "subs", e.Name())) == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		slog.Info("pruned converted subtitles", "count", removed)
	}
}
