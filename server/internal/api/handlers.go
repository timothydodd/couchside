package api

import (
	"net/http"
)

// --- status & home -----------------------------------------------------------

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	counts, err := s.db.Counts(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	jobs, err := s.db.JobCounts(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	tv := map[string]any{"configured": false}
	if s.tv != nil {
		tv = s.tv.Summary()
	}
	mediaRoots := []string{} // server paths: admins only, as tvStatus does with folders
	if currentUser(r.Context()).Admin {
		mediaRoots = s.mediaRoots(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"livetv":     tv,
		"version":    s.version,
		"providers":  s.providers.Names(),
		"tmdbKey":    s.cfg.TMDBKeySource(),
		"mediaRoots": mediaRoots,
		"counts":     counts,
		"jobs":       jobs,
		"scanEvery":  s.cfg.ScanInterval.String(),
		"workers":    s.cfg.Workers,
		"comskip":    s.worker.CommercialsAvailable(),
		"apiVersion": APIVersion,
		"features":   s.features(r.Context()),
		"transcode": map[string]any{
			"hwaccel":        s.tc.Encoder().HW,
			"requested":      s.cfg.HWAccel,
			"note":           s.tc.Encoder().Note,
			"tonemap":        s.tc.Encoder().Tonemap || s.tc.Encoder().HWTonemap,
			"gpuDecode":      s.tc.Encoder().HWDecode,
			"gpuTonemap":     s.tc.Encoder().HWTonemap,
			"tonemapFilter":  s.tc.Encoder().TonemapFilter,
			"maxSessions":    s.tc.Max(),
			"active":         len(s.tc.Sessions()),
			"encodeWorkers":  s.cfg.EncodeWorkers,
			"optimizeHeight": s.cfg.OptimizeHeight,
		},
	})
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cont, err := s.db.ContinueWatching(ctx, 20)
	if err != nil {
		writeErr(w, err)
		return
	}
	movies, err := s.db.RecentItems(ctx, "movie", 24)
	if err != nil {
		writeErr(w, err)
		return
	}
	series, err := s.db.RecentItems(ctx, "series", 24)
	if err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.db.Watchlist(ctx, 24)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"continueWatching": cont, "watchlist": list, "recentMovies": movies, "recentSeries": series})
}

// setWatchlist puts a title on the profile's "My list" (PUT) or takes it off (DELETE).
func (s *Server) setWatchlist(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.db.Item(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetWatchlist(r.Context(), id, r.Method == http.MethodPut); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// hideFromHome takes a title off the profile's Continue Watching row until
// they watch it again.
func (s *Server) hideFromHome(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.db.Item(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.HideFromHome(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
