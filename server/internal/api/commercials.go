package api

import (
	"net/http"

	"github.com/timothydodd/couchside/internal/worker"
)

// commercials reports a file's commercial breaks and whether detection is
// queued, running or failed.
func (s *Server) commercials(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	segs, done, err := s.db.Commercials(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	status, msg := "none", ""
	if done {
		status = "done"
	}
	job, err := s.db.LatestJob(r.Context(), worker.KindCommercials, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if job != nil {
		switch job.Status {
		case "queued", "running":
			status = job.Status
		case "failed":
			if !done {
				status, msg = "failed", job.Error
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": s.worker.CommercialsAvailable(),
		"status":    status,
		"error":     msg,
		"segments":  s.trimBreaks(r.Context(), segs), // what will actually be skipped
	})
}

// findCommercials queues commercial detection for a file (again).
func (s *Server) findCommercials(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !s.worker.CommercialsAvailable() {
		writeErr(w, badRequest("commercial detection needs comskip installed on the server"))
		return
	}
	f, err := s.db.File(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if f.Problem != "" {
		writeErr(w, badRequest("this file can't be read"))
		return
	}
	if err := s.worker.EnqueueCommercials(r.Context(), f.ID, f.Path); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
