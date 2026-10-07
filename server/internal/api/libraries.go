package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/worker"
)

// --- libraries ---------------------------------------------------------------

func (s *Server) listLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.db.Libraries(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, libs)
}

func (s *Server) createLibrary(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Path, Kind string }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, badRequest("name is required"))
		return
	}
	if in.Kind != "movies" && in.Kind != "tv" {
		writeErr(w, badRequest("kind must be movies or tv"))
		return
	}
	path, err := s.checkPath(in.Path)
	if err != nil {
		writeErr(w, err)
		return
	}
	id, err := s.db.CreateLibrary(r.Context(), in.Name, path, in.Kind)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, badRequest("a library already uses that folder"))
			return
		}
		writeErr(w, err)
		return
	}
	if err := s.worker.Enqueue(r.Context(), worker.KindScan, id, "Scan "+in.Name); err != nil {
		writeErr(w, err)
		return
	}
	lib, err := s.db.Library(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, lib)
}

// updateLibrary renames a library or moves it to another folder. The kind
// can't change: every item would have to be rebuilt, so that's remove and re-add.
func (s *Server) updateLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Name, Path string
		// Trickplay turns seek-bar preview thumbnails on or off; left out, it stays.
		Trickplay *bool
		// Intros turns intro detection on or off (TV libraries); left out, it stays.
		Intros *bool
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	old, err := s.db.Library(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, badRequest("name is required"))
		return
	}
	path := old.Path
	if strings.TrimSpace(in.Path) != "" && filepath.Clean(strings.TrimSpace(in.Path)) != old.Path {
		if path, err = s.checkPath(in.Path); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err := s.db.UpdateLibrary(ctx, id, in.Name, path); err != nil {
		if errors.Is(err, db.ErrPathInUse) {
			err = badRequest(err.Error())
		}
		writeErr(w, err)
		return
	}
	if path != old.Path {
		s.repointRecordings(ctx, old.Path, path)
		if err := s.worker.Enqueue(ctx, worker.KindScan, id, "Scan "+in.Name); err != nil {
			writeErr(w, err)
			return
		}
	}
	if in.Trickplay != nil && *in.Trickplay != old.Trickplay {
		if err := s.db.SetLibraryTrickplay(ctx, id, *in.Trickplay); err != nil {
			writeErr(w, err)
			return
		}
		// On: make them for the files already there. Off leaves what's made.
		if *in.Trickplay {
			if _, err := s.worker.QueueTrickplay(ctx, id); err != nil {
				writeErr(w, err)
				return
			}
		}
	}
	if in.Intros != nil && *in.Intros != old.Intros && old.Kind == "tv" {
		if err := s.db.SetLibraryIntros(ctx, id, *in.Intros); err != nil {
			writeErr(w, err)
			return
		}
		if *in.Intros {
			if _, err := s.worker.QueueIntros(ctx, id); err != nil {
				writeErr(w, err)
				return
			}
		}
	}
	lib, err := s.db.Library(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lib)
}

// repointRecordings follows DVR recordings to a library's new folder when
// the file is there, so they stay linked to their library entry.
func (s *Server) repointRecordings(ctx context.Context, oldDir, newDir string) {
	recs, err := s.db.RecordingsWithStatus(ctx, "completed")
	if err != nil {
		return
	}
	prefix := strings.TrimSuffix(oldDir, "/") + "/"
	for _, rec := range recs {
		if !strings.HasPrefix(rec.Path, prefix) {
			continue
		}
		moved := filepath.Join(newDir, strings.TrimPrefix(rec.Path, prefix))
		if _, err := os.Stat(moved); err == nil {
			_ = s.db.SetRecordingPath(ctx, rec.ID, moved)
		}
	}
}

// checkPath cleans a library path and confirms it's a readable folder inside
// the media root (when one is configured).
func (s *Server) checkPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || !filepath.IsAbs(p) {
		return "", badRequest("path must be an absolute folder path")
	}
	p = filepath.Clean(p)
	if !s.underRoot(p) {
		return "", badRequest("path must be inside " + s.cfg.MediaRoot)
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return "", badRequest("folder not found: " + p)
	}
	return p, nil
}

func (s *Server) underRoot(p string) bool {
	root := s.cfg.MediaRoot
	if root == "" {
		return true
	}
	// insideDir compares by path element with the platform's separator, so
	// D:\Media\Movies is inside D:\Media on Windows.
	within := func(r string) bool { return filepath.Clean(p) == filepath.Clean(r) || insideDir(p, r) }
	// The DVR's own folder is allowed too: it lives outside the read-only media mount.
	return within(root) || (s.tv.HasTuner() && within(s.tv.DefaultRecordingsDir()))
}

func (s *Server) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.DeleteLibrary(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) scanLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	lib, err := s.db.Library(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.worker.Enqueue(r.Context(), worker.KindScan, id, "Scan "+lib.Name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) scanAll(w http.ResponseWriter, r *http.Request) {
	s.worker.ScanAll(r.Context())
	w.WriteHeader(http.StatusAccepted)
}

// browse lists subfolders so the Libraries page can offer a folder picker.
// Only available when a media root is configured, and never escapes it.
func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MediaRoot == "" {
		writeErr(w, httpError{http.StatusNotFound, "folder browsing needs COUCHSIDE_MEDIA_ROOT"})
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		p = s.cfg.MediaRoot
	}
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) || !s.underRoot(p) {
		writeErr(w, badRequest("path must be inside "+s.cfg.MediaRoot))
		return
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		writeErr(w, badRequest("cannot read "+p))
		return
	}
	dirs := []string{}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j]) })
	var parent *string
	if p != s.cfg.MediaRoot {
		pp := filepath.Dir(p)
		parent = &pp
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "parent": parent, "dirs": dirs})
}
