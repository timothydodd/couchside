package api

import (
	"errors"
	"net/http"
	"os"
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

// getEpisode is an episode's page: the episode, its show, every copy of it,
// its guest stars and crew, and its neighbours.
func (s *Server) getEpisode(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	e, err := s.db.Episode(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	series, err := s.db.Item(ctx, e.SeriesID)
	if err != nil {
		writeErr(w, err)
		return
	}
	files, err := s.db.EpisodeCopies(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	for i := range files {
		files[i].Path = filepath.Base(files[i].Path)
	}
	cast, crew, err := s.db.EpisodeCredits(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	prev, next, err := s.db.EpisodeNeighbours(ctx, e)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"episode": e, "series": series, "files": files, "cast": cast, "crew": crew, "prev": prev, "next": next})
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
	var in struct {
		ImdbID string
		// Refresh fetches the metadata again without touching a "Fix match" choice.
		Refresh bool `json:"refresh"`
	}
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
	if !in.Refresh {
		if err := s.db.SetImdbOverride(r.Context(), id, imdb); err != nil {
			writeErr(w, err)
			return
		}
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

// mergeItems puts other titles' files under one title (admins): duplicate
// copies or bonus material for a movie, episodes for a show.
func (s *Server) mergeItems(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Into int64   `json:"into"`
		From []int64 `json:"from"`
		As   string  `json:"as"` // movies: copy | extra
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.As == "" {
		in.As = "copy"
	}
	if len(in.From) == 0 || len(in.From) > 100 {
		writeErr(w, badRequest("pick 1 to 100 titles to merge"))
		return
	}
	if err := s.db.MergeItems(r.Context(), in.Into, in.From, in.As); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, err)
			return
		}
		writeErr(w, badRequest(err.Error()))
		return
	}
	for _, id := range in.From {
		_ = os.RemoveAll(worker.ItemArtDir(s.cfg.CacheDir, id))
	}
	// Bonus material shows a frame from the file, like extras a scan finds.
	if files, err := s.db.ItemFiles(r.Context(), in.Into); err == nil {
		for _, f := range files {
			if f.Role == "extra" && !f.HasStill && f.Problem == "" {
				_ = s.worker.Enqueue(r.Context(), worker.KindStill, f.ID, "Still "+filepath.Base(f.Path))
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// setItemDetails stores details set by hand (admins): title, year, plot,
// genres and content rating. Each is optional; one left out goes back to
// the provider's, which a match fetches again when something was cleared.
func (s *Server) setItemDetails(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in db.Overrides
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" || len(t) > 200 {
			writeErr(w, badRequest("the title is 1 to 200 characters"))
			return
		}
		in.Title = &t
	}
	if in.Year != nil && (*in.Year < 1880 || *in.Year > 2100) {
		writeErr(w, badRequest("the year must be between 1880 and 2100"))
		return
	}
	if in.Plot != nil && len(*in.Plot) > 5000 {
		writeErr(w, badRequest("the description is too long"))
		return
	}
	if in.Genres != nil {
		var gs []string
		for _, g := range *in.Genres {
			if g = strings.TrimSpace(g); g != "" {
				gs = append(gs, g)
			}
		}
		if gs == nil {
			gs = []string{}
		}
		in.Genres = &gs
	}
	if in.Rated != nil {
		t := strings.TrimSpace(*in.Rated)
		in.Rated = &t
	}
	ctx := r.Context()
	item, err := s.db.Item(ctx, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	cleared, err := s.db.SetItemDetails(ctx, id, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	// Something went back to the provider's: fetch it again (keeping a fixed match).
	if cleared && item.MatchStatus == "matched" {
		_ = s.worker.Enqueue(ctx, worker.KindMatch, id, "Match "+item.ParsedTitle)
	}
	w.WriteHeader(http.StatusNoContent)
}
