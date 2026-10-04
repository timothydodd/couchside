package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/worker"
)

// --- artwork -----------------------------------------------------------------

var artKinds = map[string]string{"poster": "poster.webp", "poster-thumb": "poster-thumb.webp", "backdrop": "backdrop.webp"}

func (s *Server) itemArtwork(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	name, ok := artKinds[chi.URLParam(r, "kind")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	serveImage(w, r, filepath.Join(worker.ItemArtDir(s.cfg.CacheDir, id), name))
}

func (s *Server) fileStill(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	serveImage(w, r, worker.FileStillPath(s.cfg.CacheDir, id))
}

// serveImage serves cached artwork. URLs carry a ?v= version from the item's
// updated_at, so the browser can cache them for a long time.
func serveImage(w http.ResponseWriter, r *http.Request, path string) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeFile(w, r, path)
}
