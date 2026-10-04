package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
		p, err := s.db.OptimizedPath(r.Context(), id)
		if err != nil || p == "" {
			writeErr(w, httpError{http.StatusNotFound, "no optimized version"})
			return
		}
		f.Path = p
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
