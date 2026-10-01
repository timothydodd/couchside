package api

import (
	"context"
	"math"
	"net/http"
	"strconv"

	"github.com/timothydodd/couchside/internal/db"
)

// Commercial skipping leaves a little of each detected break in: comskip's
// edges are often a second off, and cutting into the show is worse than
// seeing a moment of an ad.
const (
	settingSkipAfterStart = "commercials.skip_after_start" // seconds into a break before skipping
	settingSkipBeforeEnd  = "commercials.skip_before_end"  // seconds before its end to land
	defaultSkipTrim       = 1.0
	maxSkipTrim           = 15.0
	maxPadding            = 30 * 60
)

func (s *Server) floatSetting(ctx context.Context, key string, def float64) float64 {
	if v, err := s.db.Setting(ctx, key); err == nil && v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			return f
		}
	}
	return def
}

// trimBreaks shrinks each break by the skip settings and drops any left too short to skip.
func (s *Server) trimBreaks(ctx context.Context, segs []db.Segment) []db.Segment {
	after := s.floatSetting(ctx, settingSkipAfterStart, defaultSkipTrim)
	before := s.floatSetting(ctx, settingSkipBeforeEnd, defaultSkipTrim)
	out := make([]db.Segment, 0, len(segs))
	for _, g := range segs {
		g.Start += after
		g.End -= before
		if g.End-g.Start >= 1 {
			out = append(out, g)
		}
	}
	return out
}

type timing struct {
	PadBefore      int64   `json:"padBefore"` // seconds a recording starts early
	PadAfter       int64   `json:"padAfter"`  // and runs late
	SkipAfterStart float64 `json:"skipAfterStart"`
	SkipBeforeEnd  float64 `json:"skipBeforeEnd"`
	DVR            bool    `json:"dvr"` // padding only matters with a tuner
}

func (s *Server) getTiming(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.currentTiming(r.Context()))
}

func (s *Server) currentTiming(ctx context.Context) timing {
	t := timing{
		SkipAfterStart: s.floatSetting(ctx, settingSkipAfterStart, defaultSkipTrim),
		SkipBeforeEnd:  s.floatSetting(ctx, settingSkipBeforeEnd, defaultSkipTrim),
		DVR:            s.tv != nil,
	}
	if s.tv != nil {
		t.PadBefore, t.PadAfter = s.tv.Padding(ctx)
	} else {
		t.PadBefore, t.PadAfter = int64(s.cfg.PadBefore.Seconds()), int64(s.cfg.PadAfter.Seconds())
	}
	return t
}

func (s *Server) saveTiming(w http.ResponseWriter, r *http.Request) {
	var in timing
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	switch {
	case in.PadBefore < 0 || in.PadAfter < 0 || in.PadBefore > maxPadding || in.PadAfter > maxPadding:
		writeErr(w, badRequest("recording padding must be between 0 and 30 minutes"))
		return
	case in.SkipAfterStart < 0 || in.SkipBeforeEnd < 0 || in.SkipAfterStart > maxSkipTrim || in.SkipBeforeEnd > maxSkipTrim,
		math.IsNaN(in.SkipAfterStart) || math.IsNaN(in.SkipBeforeEnd):
		writeErr(w, badRequest("commercial skip offsets must be between 0 and 15 seconds"))
		return
	}
	ctx := r.Context()
	for k, v := range map[string]float64{settingSkipAfterStart: in.SkipAfterStart, settingSkipBeforeEnd: in.SkipBeforeEnd} {
		if err := s.db.SetSetting(ctx, k, strconv.FormatFloat(v, 'f', -1, 64)); err != nil {
			writeErr(w, err)
			return
		}
	}
	if s.tv != nil {
		if err := s.tv.SetPadding(ctx, in.PadBefore, in.PadAfter); err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, s.currentTiming(ctx))
}
