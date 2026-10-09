package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/fsx"
	"github.com/timothydodd/couchside/internal/parse"
	"github.com/timothydodd/couchside/internal/worker"
)

// --- files & playback --------------------------------------------------------

func (s *Server) playInfo(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.db.PlayInfo(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) saveProgress(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Position, Duration float64
		State              string // playing | paused | stopped (older clients send none)
		Mode               string // how it's playing, shown on the Settings page
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Position < 0 || in.Duration < 0 {
		writeErr(w, badRequest("position and duration must be positive"))
		return
	}
	if in.State == "stopped" {
		s.presence.setPlaying(r, nil)
	} else {
		s.presence.setPlaying(r, &playing{Kind: "file", FileID: id, Position: in.Position, Duration: in.Duration,
			Mode: clip(in.Mode, 60), Paused: in.State == "paused"})
	}
	if err := s.db.SaveProgress(r.Context(), id, in.Position, in.Duration); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fileWatched(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct{ Watched bool }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetWatched(r.Context(), []int64{id}, in.Watched); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var mimeTypes = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
	".mkv": "video/x-matroska", ".avi": "video/x-msvideo", ".ts": "video/mp2t", ".m2ts": "video/mp2t",
}

// stream serves the original file with byte-range support (direct play).
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
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
	if r.URL.Query().Get("version") == "optimized" {
		if msg := ProblemMessage(f.Problem); msg != "" {
			writeErr(w, httpError{http.StatusUnprocessableEntity, msg})
			return
		}
		p, err := s.db.OptimizedPath(r.Context(), id)
		if err != nil || p == "" {
			writeErr(w, httpError{http.StatusNotFound, "no optimized version"})
			return
		}
		f.Path = worker.ResolveCache(s.cfg.CacheDir, p)
	} else if err := s.checkMedia(r.Context(), f); err != nil {
		writeErr(w, err)
		return
	}
	fh, err := os.Open(f.Path)
	if err != nil {
		writeErr(w, httpError{http.StatusNotFound, "file is missing on disk; rescan the library"})
		return
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		writeErr(w, err)
		return
	}
	if ct, ok := mimeTypes[strings.ToLower(filepath.Ext(f.Path))]; ok {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, filepath.Base(f.Path), st.ModTime(), fh)
}

// errMissing is what a file that can't be served answers: missing, or a
// link planted in the library. Both look the same from outside.
var errMissing = httpError{http.StatusNotFound, "file is missing on disk; rescan the library"}

// checkMedia refuses a file the scanner flagged as unplayable, and one whose
// path leads (through a link) outside its library to anything but a video,
// before its bytes are served or handed to ffmpeg.
func (s *Server) checkMedia(ctx context.Context, f db.File) error {
	if msg := ProblemMessage(f.Problem); msg != "" {
		return httpError{http.StatusUnprocessableEntity, msg}
	}
	return s.checkLink(ctx, f)
}

// checkLink is checkMedia without the playability check, for routes that
// read a file's tracks or sidecars.
func (s *Server) checkLink(ctx context.Context, f db.File) error {
	if s.linkOK.recent(f.ID, f.Path) {
		return nil
	}
	lib, err := s.db.Library(ctx, f.LibraryID)
	if err != nil {
		return err
	}
	if err := fsx.Allowed(f.Path, lib.Path, parse.IsVideo); err != nil {
		if errors.Is(err, fsx.ErrRefused) {
			slog.Warn("refused a file that links outside its library", "file", f.ID, "path", f.Path)
		}
		return errMissing
	}
	s.linkOK.remember(f.ID, f.Path)
	return nil
}

// linkChecks remembers files that passed checkLink for a minute. A player
// sends hundreds of range requests for one film, and resolving the path
// through links costs several round trips on a network share.
type linkChecks struct {
	mu   sync.Mutex
	seen map[int64]linkCheck
}

type linkCheck struct {
	path string
	at   time.Time
}

const linkCheckTTL = time.Minute

func (c *linkChecks) recent(id int64, path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.seen[id]
	return ok && e.path == path && time.Since(e.at) < linkCheckTTL
}

func (c *linkChecks) remember(id int64, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen == nil || len(c.seen) > 1000 {
		c.seen = map[int64]linkCheck{} // a handful are playing at any time
	}
	c.seen[id] = linkCheck{path, time.Now()}
}
