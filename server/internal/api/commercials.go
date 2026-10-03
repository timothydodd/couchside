package api

import (
	"net/http"
	"strings"

	"github.com/timothydodd/couchside/internal/db"
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
	dismissed, err := s.db.CommercialDismissals(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": s.worker.CommercialsAvailable(),
		"status":    status,
		"error":     msg,
		"segments":  withoutDismissed(s.trimBreaks(r.Context(), segs), dismissed), // what will actually be skipped
		"dismissed": dismissed,
	})
}

// withoutDismissed drops breaks marked "Not a commercial": any break that a
// dismissal covers for at least half its length, so one survives detection
// running again and moving the edges a little.
func withoutDismissed(segs, dismissed []db.Segment) []db.Segment {
	out := make([]db.Segment, 0, len(segs))
next:
	for _, g := range segs {
		for _, d := range dismissed {
			if overlap := min(g.End, d.End) - max(g.Start, d.Start); overlap >= (g.End-g.Start)/2 {
				continue next
			}
		}
		out = append(out, g)
	}
	return out
}

// dismissCommercial marks a break "Not a commercial", or with restore, undoes it.
func (s *Server) dismissCommercial(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in struct {
		Start   float64 `json:"start"`
		End     float64 `json:"end"`
		Restore bool    `json:"restore"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Start < 0 || in.End <= in.Start {
		writeErr(w, badRequest("a break needs a start before its end"))
		return
	}
	if _, err := s.db.File(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	seg := db.Segment{Start: in.Start, End: in.End}
	if in.Restore {
		err = s.db.RestoreCommercial(r.Context(), id, seg)
	} else {
		err = s.db.DismissCommercial(r.Context(), id, seg)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
