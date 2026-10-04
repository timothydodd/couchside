package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/probe"
	"github.com/timothydodd/couchside/internal/worker"
)

// A file's intro and end credits, for the player's Skip intro and Skip
// credits buttons (table file_segments). They come from the file's chapter
// names, read the first time the file is played; from the opening a season's
// episodes share (worker/intros.go); or from an admin marking them.

// segments lists a file's intro and credits.
func (s *Server) segments(w http.ResponseWriter, r *http.Request) {
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
	s.readChapters(r.Context(), f)
	segs, err := s.db.FileSegments(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"segments": segs})
}

// readChapters looks for an intro and credits in the file's chapters, once
// per version of the file. A failure is logged and tried again next time.
func (s *Server) readChapters(ctx context.Context, f db.File) {
	if f.Problem != "" {
		return
	}
	if done, err := s.db.ChaptersChecked(ctx, f.ID, f.Size, f.Mtime); err != nil || done {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	chapters, err := probe.Chapters(ctx, s.cfg.FFprobe, f.Path)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("read chapters", "file", f.Path, "err", err)
		}
		return
	}
	dur := 0.0
	if f.DurationSec != nil {
		dur = *f.DurationSec
	}
	var segs []db.MarkedSegment
	intro, credits := probe.IntroAndCredits(chapters, dur)
	if intro != nil {
		segs = append(segs, db.MarkedSegment{Kind: "intro", Start: intro.Start, End: intro.End})
	}
	if credits != nil {
		segs = append(segs, db.MarkedSegment{Kind: "credits", Start: credits.Start, End: credits.End})
	}
	if err := s.db.SetChapterSegments(ctx, f.ID, f.Size, f.Mtime, segs); err != nil {
		slog.Warn("store chapter segments", "file", f.ID, "err", err)
	}
}

// findIntros queues intro detection for a series (admin; the title's "⋯" menu).
func (s *Server) findIntros(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := s.db.Item(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if item.Kind != "series" {
		writeErr(w, badRequest("intros are found by comparing a season's episodes; this isn't a series"))
		return
	}
	if err := s.worker.Enqueue(r.Context(), worker.KindIntros, id, "Find intros "+item.Title); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": 1})
}

// setSegment is an admin marking (PUT) or clearing (DELETE) a file's intro
// or credits by hand.
func (s *Server) setSegment(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	kind := chi.URLParam(r, "kind")
	if kind != "intro" && kind != "credits" {
		writeErr(w, badRequest("kind must be intro or credits"))
		return
	}
	f, err := s.db.File(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if r.Method == http.MethodDelete {
		if err := s.db.DeleteSegment(r.Context(), id, kind); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var in struct{ Start, End float64 }
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if f.DurationSec != nil && in.End > *f.DurationSec {
		in.End = *f.DurationSec
	}
	if in.Start < 0 || in.End-in.Start < 1 {
		writeErr(w, badRequest("the end must be at least a second after the start"))
		return
	}
	if err := s.db.SetSegment(r.Context(), id, db.MarkedSegment{Kind: kind, Start: in.Start, End: in.End, Source: "manual"}); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
