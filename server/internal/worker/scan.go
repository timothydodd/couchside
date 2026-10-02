package worker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/parse"
	"github.com/timothydodd/couchside/internal/probe"
)

// Folders NAS boxes and download tools litter libraries with.
var skipDirs = map[string]bool{"@eadir": true, "#recycle": true, "$recycle.bin": true, ".trash": true, "lost+found": true}

var reSample = regexp.MustCompile(`(?i)(^|[^a-z])sample([^a-z]|$)`)

// rePartFile matches the pieces a DVR recording writes before joining them.
var rePartFile = regexp.MustCompile(`\.(part\d+|joining)\.ts$`)

func (w *Worker) scan(ctx context.Context, libID int64) error {
	lib, err := w.db.Library(ctx, libID)
	if err != nil {
		return err
	}
	if st, err := os.Stat(lib.Path); err != nil || !st.IsDir() {
		return fmt.Errorf("library path %s is not readable: %v", lib.Path, err)
	}
	// Every file visited gets last_seen = start; anything older is gone.
	start := time.Now().Unix()
	var added, changed, skipped int

	walkErr := filepath.WalkDir(lib.Path, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			slog.Warn("scan: skipping unreadable path", "path", path, "err", err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != lib.Path && (strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)]) {
				return fs.SkipDir
			}
			// Movie extras are indexed as extras of their movie; TV has no use for them.
			if _, extras := parse.ExtrasDir(name); extras && lib.Kind == "tv" && path != lib.Path {
				return fs.SkipDir
			}
			return nil
		}
		if !parse.IsVideo(name) || strings.HasPrefix(name, ".") {
			return nil
		}
		// A recording in progress isn't an episode yet; its parts are joined
		// into the final file, which the scan after it finishes picks up.
		if m := rePartFile.FindStringIndex(path); m != nil {
			if active, err := w.db.RecordingPathActive(ctx, path[:m[0]]+".ts"); err != nil || active {
				return err
			}
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if reSample.MatchString(name) && info.Size() < 300<<20 {
			return nil
		}
		stamp, err := w.db.FileStamp(ctx, path)
		if err != nil {
			return err
		}
		if stamp != nil && stamp.Size == info.Size() && stamp.Mtime == info.ModTime().Unix() && !w.parseChanged(lib, path, stamp) {
			if err := w.refreshRole(ctx, lib, path, stamp); err != nil {
				return err
			}
			return w.db.TouchFile(ctx, stamp.ID, start)
		}
		ok, err := w.indexFile(ctx, lib, path, info, start)
		if err != nil {
			return err
		}
		switch {
		case !ok:
			skipped++
		case stamp == nil:
			added++
		default:
			changed++
		}
		return nil
	})
	if walkErr != nil {
		// Don't prune after a partial walk: missing files may just be unvisited.
		return walkErr
	}
	removed, err := w.db.PruneLibrary(ctx, lib.ID, start)
	if err != nil {
		return err
	}
	slog.Info("scan complete", "library", lib.Name, "added", added, "changed", changed, "removed", removed, "skipped", skipped)
	return w.db.MarkLibraryScanned(ctx, lib.ID, time.Now().Unix())
}

// parseChanged reports whether today's parser reads an unchanged file
// differently than when it was indexed (e.g. "THE BURBS_t03" is now "THE
// BURBS"), so the file is re-indexed under the right title or episode.
func (w *Worker) parseChanged(lib db.Library, path string, st *db.FileStamp) bool {
	if lib.Kind == "tv" {
		rel, _ := filepath.Rel(lib.Path, path)
		r, ok := parse.Episode(rel)
		return ok && (r.Title != st.ParsedTitle || r.Year != st.ParsedYear || r.Season != st.Season || r.Episode != st.Episode)
	}
	r := parse.Movie(relPath(lib, path))
	return r.Title != st.ParsedTitle || r.Year != st.ParsedYear
}

func relPath(lib db.Library, path string) string {
	rel, err := filepath.Rel(lib.Path, path)
	if err != nil {
		return path
	}
	return rel
}

// refreshRole brings an unchanged movie file's guessed role (part, extra) up
// to date with the parser, so detection reaches files indexed before it.
func (w *Worker) refreshRole(ctx context.Context, lib db.Library, path string, st *db.FileStamp) error {
	if lib.Kind == "tv" || st.RolePinned {
		return nil
	}
	r := parse.MovieRole(relPath(lib, path), st.ParsedTitle)
	if r.Kind == st.Role && r.Part == st.PartNo && r.Extra == st.ExtraTitle {
		return nil
	}
	if err := w.db.SetDetectedRole(ctx, st.ID, r.Kind, r.Part, r.Extra); err != nil {
		return err
	}
	if r.Kind == "extra" {
		return w.Enqueue(ctx, KindStill, st.ID, "Still "+filepath.Base(path))
	}
	return nil
}

// indexFile parses, probes and records one file. It returns false for files
// that can't be placed (a TV file with no episode number).
func (w *Worker) indexFile(ctx context.Context, lib db.Library, path string, info fs.FileInfo, seen int64) (bool, error) {
	f := db.File{LibraryID: lib.ID, Path: path, Size: info.Size(), Mtime: info.ModTime().Unix()}
	var (
		itemID  int64
		created bool
		err     error
		label   string
		role    = parse.Role{Kind: "copy"}
	)
	if lib.Kind == "tv" {
		rel, _ := filepath.Rel(lib.Path, path)
		r, ok := parse.Episode(rel)
		if !ok {
			slog.Info("scan: no episode number, skipping", "path", path)
			return false, nil
		}
		if itemID, created, err = w.db.EnsureItem(ctx, lib.ID, "series", r.Title, r.Year); err != nil {
			return false, err
		}
		epID, err := w.db.EnsureEpisode(ctx, itemID, r.Season, r.Episode, r.EpisodeTitle, r.AirDate)
		if err != nil {
			return false, err
		}
		f.EpisodeID = &epID
		label = r.Title
	} else {
		rel := relPath(lib, path)
		r := parse.Movie(rel)
		if itemID, created, err = w.db.EnsureItem(ctx, lib.ID, "movie", r.Title, r.Year); err != nil {
			return false, err
		}
		label = r.Title
		role = parse.MovieRole(rel, r.Title)
	}
	f.MediaItemID = itemID

	if p, err := probe.Probe(ctx, w.cfg.FFprobe, path); err != nil {
		if errors.Is(err, context.Canceled) {
			return false, err
		}
		slog.Warn("scan: file is unreadable (corrupt or incomplete)", "path", path, "err", err)
		f.Container = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		f.Problem = "unreadable"
	} else {
		f.DurationSec, f.Container, f.VideoCodec, f.AudioCodec = p.DurationSec, p.Container, p.VideoCodec, p.AudioCodec
		f.Width, f.Height, f.AudioTracks, f.SubtitleTracks = p.Width, p.Height, p.AudioTracks, p.SubtitleTracks
		if p.VideoCodec == "" {
			// Usually DRM-protected iTunes purchases: the video stream has no codec ffmpeg can decode.
			slog.Warn("scan: file has no decodable video (DRM-protected?)", "path", path)
			f.Problem = "no-video"
		}
	}

	fileID, err := w.db.UpsertFile(ctx, f, seen)
	if err != nil {
		return false, err
	}
	if lib.Kind != "tv" {
		if err := w.db.SetDetectedRole(ctx, fileID, role.Kind, role.Part, role.Extra); err != nil {
			return false, err
		}
	}
	// A changed file makes its optimized copy stale; cleanupOptimized deletes it on disk.
	if old, err := w.db.DeleteOptimized(ctx, fileID); err == nil && old != "" {
		_ = os.Remove(old)
	}
	if created {
		if err := w.Enqueue(ctx, KindMatch, itemID, "Match "+label); err != nil {
			return false, err
		}
	}
	// Episodes and extras are shown with a frame from the file.
	if (f.EpisodeID != nil || role.Kind == "extra") && f.Problem == "" {
		if err := w.Enqueue(ctx, KindStill, fileID, "Still "+filepath.Base(path)); err != nil {
			return false, err
		}
	}
	if w.comskip != "" && f.Problem == "" {
		if err := w.autoCommercials(ctx, fileID, path); err != nil {
			return false, err
		}
	}
	return true, nil
}

// autoCommercials queues commercial detection for a finished DVR recording
// that hasn't been analysed in its current form.
func (w *Worker) autoCommercials(ctx context.Context, fileID int64, path string) error {
	if rec, err := w.db.IsRecording(ctx, path); err != nil || !rec {
		return err
	}
	if _, done, err := w.db.Commercials(ctx, fileID); err != nil || done {
		return err
	}
	return w.EnqueueCommercials(ctx, fileID, path)
}
