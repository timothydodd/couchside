package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/probe"
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
// "Movie.en.forced.srt".
func sidecars(videoPath string) []string {
	dir := filepath.Dir(videoPath)
	stem := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && sidecarExts[strings.ToLower(filepath.Ext(n))] && strings.HasPrefix(n, stem+".") {
			out = append(out, filepath.Join(dir, n))
		}
	}
	return out
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
		writeErr(w, httpError{http.StatusBadGateway, err.Error()})
		return
	}
	subs := []subtitleTrack{}
	for _, sub := range st.Subtitles {
		subs = append(subs, subtitleTrack{Key: fmt.Sprintf("s%d", sub.Index), Language: sub.Language, Title: sub.Title,
			Codec: sub.Codec, Text: sub.Text, Forced: sub.Forced, Default: sub.Default, Index: sub.Index})
	}
	stem := strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
	for i, p := range sidecars(f.Path) {
		// "Movie.en.forced.srt" → language "en", title "forced"
		mid := strings.Split(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), stem), filepath.Ext(p)), ".")
		t := subtitleTrack{Key: fmt.Sprintf("x%d", i), Codec: strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), "."),
			Text: true, External: true, Index: -1}
		for _, part := range mid {
			switch l := strings.ToLower(part); {
			case l == "":
			case l == "forced":
				t.Forced = true
			case l == "sdh" || l == "cc" || l == "hi":
				t.Title = strings.ToUpper(part)
			case t.Language == "" && len(l) <= 3:
				t.Language = l
			default:
				t.Title = part
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
	subLocks sync.Map // cache path → *sync.Mutex, so one conversion runs per track
)

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
	}
	cache := filepath.Join(s.cfg.CacheDir, "subs", fmt.Sprintf("%d-%s%d-%d%s.vtt", id, m[1], n, f.Mtime, chunkTag))
	lk, _ := subLocks.LoadOrStore(cache, &sync.Mutex{})
	mu := lk.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if _, err := os.Stat(cache); err != nil {
		if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
			writeErr(w, err)
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
			msg := strings.TrimSpace(string(out))
			if ctx.Err() != nil {
				msg = "timed out reading the file"
			} else if msg == "" {
				msg = err.Error()
			}
			writeErr(w, httpError{http.StatusUnprocessableEntity, "couldn't convert subtitles: " + msg})
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
