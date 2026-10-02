package api

import (
	"net/http"
	"strings"

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

// broadcastContainers are the files the player offers detection for
// (BROADCAST in web PlayerPage).
var broadcastContainers = map[string]bool{"ts": true, "mpg": true, "mpeg": true, "wtv": true}

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
	if !currentUser(r.Context()).Admin {
		// What the player offers: a first look at a broadcast recording.
		// Running comskip again, or on anything else, is an admin's call.
		if !broadcastContainers[strings.ToLower(f.Container)] {
			writeErr(w, forbidden("commercial detection is for TV recordings"))
			return
		}
		if _, done, err := s.db.Commercials(r.Context(), f.ID); err != nil {
			writeErr(w, err)
			return
		} else if done {
			writeErr(w, forbidden("only an admin can run detection again"))
			return
		}
	}
	if err := s.worker.EnqueueCommercials(r.Context(), f.ID, f.Path); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// findItemCommercials queues commercial detection for every file of a movie
// or series: only ones not checked yet, or all of them with {"redo": true}.
func (s *Server) findItemCommercials(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct{ Redo bool }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if !s.worker.CommercialsAvailable() {
		writeErr(w, badRequest("commercial detection needs comskip installed on the server"))
		return
	}
	ctx := r.Context()
	if _, err := s.db.Item(ctx, id); err != nil {
		writeErr(w, err)
		return
	}
	files, err := s.db.CommercialCandidates(ctx, id, in.Redo)
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, f := range files {
		if err := s.worker.EnqueueCommercials(ctx, f.ID, f.Path); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"queued": len(files)})
}
