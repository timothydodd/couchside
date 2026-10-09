package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/netshare"
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
	path, err := s.checkPath(r.Context(), in.Path)
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
		if path, err = s.checkPath(r.Context(), in.Path); err != nil {
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
// a media location.
func (s *Server) checkPath(ctx context.Context, p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || !filepath.IsAbs(p) {
		return "", badRequest("path must be an absolute folder path")
	}
	p = filepath.Clean(p)
	if !s.underRoot(ctx, p) {
		return "", errOutsideLocations(p)
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return "", badRequest("folder not found: " + p)
	}
	return p, nil
}

// underRoot says p is in a media location, or the DVR's own folder (which
// lives outside the read-only media mount).
func (s *Server) underRoot(ctx context.Context, p string) bool {
	return within(p, s.mediaRoots(ctx)) || (s.tv != nil && s.tv.HasTuner() && within(p, []string{s.tv.DefaultRecordingsDir()}))
}

func (s *Server) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	pr, err := s.db.DeleteLibrary(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	worker.TidyCache(s.cfg.CacheDir, pr)
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

type folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// browse lists subfolders for the folder pickers (libraries, the DVR and
// filler folders, and adding a media location). Admins only. Inside the
// media locations, unless all=1 (adding a location); no path lists the
// starting points: the locations, or with all=1 the drives on Windows (plus
// shares already in use) and / elsewhere. Paths come back whole, so the
// client never joins them with the wrong separator.
func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1"
	roots := s.mediaRoots(r.Context())
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		start := startFolders(roots)
		if !all {
			start = make([]folder, 0, len(roots))
			for _, root := range roots {
				start = append(start, folder{root, root})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": "", "parent": nil, "dirs": start})
		return
	}
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) {
		writeErr(w, badRequest("enter a full folder path"))
		return
	}
	if !all && !within(p, roots) {
		writeErr(w, errOutsideLocations(p))
		return
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		writeErr(w, badRequest("can't open "+p))
		return
	}
	dirs := []folder{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "$") {
			continue
		}
		// Follows links, which a NAS share or a mount point may be.
		if st, err := os.Stat(filepath.Join(p, e.Name())); err == nil && st.IsDir() {
			dirs = append(dirs, folder{e.Name(), filepath.Join(p, e.Name())})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })
	// Up a level, or back to the starting points from a location (or a
	// drive or share's own root).
	parent := filepath.Dir(p)
	if parent == p || (!all && !within(parent, roots)) {
		parent = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "parent": parent, "dirs": dirs})
}

// startFolders are where browsing for a new location starts: the drives on
// Windows (plus shares already in use), / elsewhere.
func startFolders(roots []string) []folder {
	if runtime.GOOS != "windows" {
		return []folder{{"/", "/"}}
	}
	var out []folder
	for c := 'A'; c <= 'Z'; c++ {
		d := string(c) + `:\`
		if _, err := os.Stat(d); err == nil {
			out = append(out, folder{string(c) + ":", d})
		}
	}
	seen := map[string]bool{}
	for _, r := range roots {
		if root, err := netshare.Root(r); err == nil && !seen[strings.ToLower(root)] {
			seen[strings.ToLower(root)] = true
			out = append(out, folder{root, root})
		}
	}
	return out
}
