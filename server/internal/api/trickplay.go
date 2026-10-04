package api

import (
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/worker"
)

// trickplay describes a file's seek-bar preview thumbnails (see
// worker.TrickIndex), or answers 404 when it has none.
func (s *Server) trickplay(w http.ResponseWriter, r *http.Request) {
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
	ix := worker.Trickplay(s.cfg.CacheDir, f)
	if ix == nil {
		writeErr(w, httpError{http.StatusNotFound, "no preview thumbnails for this file"})
		return
	}
	writeJSON(w, http.StatusOK, ix)
}

// trickplayFile serves one sheet of thumbnails ("0.jpg", "1.jpg", …) or the
// Roku's "index.bif".
func (s *Server) trickplayFile(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	name := chi.URLParam(r, "name")
	if name != "index.bif" && !worker.TrickSheetName(name) {
		http.NotFound(w, r)
		return
	}
	f, err := s.db.File(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if worker.Trickplay(s.cfg.CacheDir, f) == nil {
		http.NotFound(w, r)
		return
	}
	// Made from the file as it is: a changed file gets new thumbnails and a
	// new v= in the URL the client builds, so these can be kept.
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeFile(w, r, filepath.Join(worker.TrickDir(s.cfg.CacheDir, id), name))
}
