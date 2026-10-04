package api

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/metadata"
	"github.com/timothydodd/couchside/internal/worker"
)

// --- items -------------------------------------------------------------------

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind != "movie" && kind != "series" {
		writeErr(w, badRequest("kind must be movie or series"))
		return
	}
	items, err := s.db.Items(r.Context(), kind)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type season struct {
	Season   int             `json:"season"`
	Episodes []db.EpisodeRow `json:"episodes"`
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	item, err := s.db.Item(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	files, err := s.db.ItemFiles(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	for i := range files {
		files[i].Path = filepath.Base(files[i].Path) // don't leak server paths to the UI
	}
	cast, crew, err := s.db.ItemCredits(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	// The copy this profile chose to watch (0 = none chosen: the best one).
	version, err := s.db.PreferredVersion(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := map[string]any{"item": item, "files": files, "cast": cast, "crew": crew, "versionFileId": version}
	if item.Kind == "series" {
		eps, err := s.db.SeriesEpisodes(ctx, id)
		if err != nil {
			writeErr(w, err)
			return
		}
		seasons := []season{}
		for _, e := range eps {
			if len(seasons) == 0 || seasons[len(seasons)-1].Season != e.Season {
				seasons = append(seasons, season{Season: e.Season})
			}
			seasons[len(seasons)-1].Episodes = append(seasons[len(seasons)-1].Episodes, e)
		}
		out["seasons"] = seasons
		out["files"] = []db.File{}
	}
	writeJSON(w, http.StatusOK, out)
}

// setVersion remembers which copy of a title this profile watches.
func (s *Server) setVersion(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		FileID int64 `json:"fileId"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	// Through File, so a copy the profile may not see can't be chosen.
	if f, err := s.db.File(r.Context(), in.FileID); err != nil || f.MediaItemID != id {
		writeErr(w, db.ErrNotFound)
		return
	}
	if err := s.db.SetPreferredVersion(r.Context(), id, in.FileID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var reImdb = regexp.MustCompile(`tt\d{5,10}`)

// rematch re-runs metadata matching, optionally pinned to an IMDb id or URL.
func (s *Server) rematch(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct{ ImdbID string }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	imdb := ""
	if strings.TrimSpace(in.ImdbID) != "" {
		// An IMDb id or URL, or a TMDB reference or URL for titles IMDb doesn't have.
		if imdb = reImdb.FindString(in.ImdbID); imdb == "" {
			imdb = metadata.TMDBID(in.ImdbID)
		}
		if imdb == "" {
			writeErr(w, badRequest("that doesn't look like an IMDb id (tt1234567), an IMDb URL or a TMDB URL"))
			return
		}
	}
	item, err := s.db.Item(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetImdbOverride(r.Context(), id, imdb); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.worker.Enqueue(r.Context(), worker.KindMatch, id, "Match "+item.ParsedTitle); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) itemWatched(w http.ResponseWriter, r *http.Request) {
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
	files, err := s.db.FeatureFiles(r.Context(), id) // extras keep their own state
	if err != nil {
		writeErr(w, err)
		return
	}
	ids := make([]int64, len(files))
	for i, f := range files {
		ids[i] = f.ID
	}
	if err := s.db.SetWatched(r.Context(), ids, in.Watched); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
