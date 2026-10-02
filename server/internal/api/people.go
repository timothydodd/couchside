package api

import (
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/timothydodd/couchside/internal/worker"
)

// person is someone's page: who they are and what of theirs is in the library.
func (s *Server) person(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	p, err := s.db.Person(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	items, err := s.db.PersonItems(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"person": p, "items": items})
}

// personPhoto serves a person's photo, fetching it from TMDB on first use.
// Open like the rest of the artwork.
func (s *Server) personPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	path := worker.PersonPhotoPath(s.cfg.CacheDir, id)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		p, err := s.db.Person(r.Context(), id)
		if err != nil || p.ProfilePath == "" {
			http.NotFound(w, r)
			return
		}
		if path, err = s.worker.PersonPhoto(r.Context(), id, p.ProfilePath); err != nil {
			slog.Warn("person photo", "person", id, "err", err)
			http.NotFound(w, r)
			return
		}
	}
	serveImage(w, r, path)
}

// rematchLibrary matches every item in a library again: after switching
// providers (OMDb to TMDB), or to pick up backdrops and cast. Pinned matches
// stay pinned.
func (s *Server) rematchLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	items, err := s.db.ItemIDsInLibrary(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, it := range items {
		if err := s.worker.Enqueue(ctx, worker.KindMatch, it.ID, "Match "+it.Title); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": len(items)})
}
