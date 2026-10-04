package api

import (
	"net/http"
	"strconv"
)

// --- jobs --------------------------------------------------------------------

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	jobs, err := s.db.RecentJobs(r.Context(), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	counts, err := s.db.JobCounts(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "counts": counts})
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.RetryJob(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	s.worker.Wake()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) clearJobs(w http.ResponseWriter, r *http.Request) {
	if err := s.db.ClearFinishedJobs(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
