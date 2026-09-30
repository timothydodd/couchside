package api

import (
	"net/http"
	"strconv"

	"github.com/timothydodd/couchside/internal/db"
)

func (s *Server) rulesList(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	rules, err := s.db.Rules(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

// ruleOptions returns what the "Record series" form needs for a program:
// the existing rule (if any) and library shows to compare against.
func (s *Server) ruleOptions(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	pid, _ := strconv.ParseInt(r.URL.Query().Get("programId"), 10, 64)
	p, err := s.db.Program(r.Context(), pid)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := map[string]any{"candidates": s.tv.LibraryMatchesFor(r.Context(), p), "rule": nil}
	if p.RuleID != nil {
		if rule, err := s.db.Rule(r.Context(), *p.RuleID); err == nil {
			out["rule"] = rule
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type ruleInput struct {
	ProgramID   int64  `json:"programId"`
	Mode        string `json:"mode"`
	Channel     string `json:"channel"`
	MediaItemID *int64 `json:"mediaItemId"`
	KeepLast    int    `json:"keepLast"`
	Enabled     *bool  `json:"enabled"`
}

func (s *Server) ruleCreate(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	var in ruleInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	rule, sum, err := s.tv.CreateRule(r.Context(), in.ProgramID, in.Mode, in.Channel, in.MediaItemID, in.KeepLast)
	if err != nil {
		if err == db.ErrNotFound {
			writeErr(w, err)
			return
		}
		writeErr(w, badRequest(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"rule": rule, "summary": sum})
}

func (s *Server) ruleUpdate(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	cur, err := s.db.Rule(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in ruleInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Mode != "" {
		cur.Mode = in.Mode
	}
	cur.Channel = in.Channel
	cur.MediaItemID = in.MediaItemID
	cur.KeepLast = max(0, in.KeepLast)
	if in.Enabled != nil {
		cur.Enabled = *in.Enabled
	}
	rule, sum, err := s.tv.UpdateRule(r.Context(), cur)
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rule": rule, "summary": sum})
}

func (s *Server) ruleDelete(w http.ResponseWriter, r *http.Request) {
	if s.tv == nil {
		writeErr(w, errNoTuner)
		return
	}
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.tv.DeleteRule(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
