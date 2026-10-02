package api

import (
	"net/http"
	"strconv"

	"github.com/timothydodd/couchside/internal/db"
)

func (s *Server) rulesList(w http.ResponseWriter, r *http.Request) {
	if !s.tv.HasTuner() {
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
	if !s.tv.HasTuner() {
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
	if !s.tv.HasTuner() {
		writeErr(w, errNoTuner)
		return
	}
	var in ruleInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	u := currentUser(ctx)
	// A series has one rule; creating it again updates it, so that has to be yours.
	if p, err := s.db.Program(ctx, in.ProgramID); err == nil && p.SeriesID != "" {
		exists, owner, err := s.db.RuleOwnerForSeries(ctx, p.SeriesID)
		if err != nil {
			writeErr(w, err)
			return
		}
		if exists && !u.mayManage(owner) {
			writeErr(w, forbidden("someone else already records this series"))
			return
		}
	}
	rule, sum, err := s.tv.CreateRule(ctx, in.ProgramID, in.Mode, in.Channel, in.MediaItemID, in.KeepLast)
	if err != nil {
		if err == db.ErrNotFound {
			writeErr(w, err)
			return
		}
		writeErr(w, badRequest(err.Error()))
		return
	}
	if err := s.db.SetRuleOwner(ctx, rule.ID, u.ID); err != nil {
		writeErr(w, err)
		return
	}
	rule.OwnerID = u.ID
	writeJSON(w, http.StatusCreated, map[string]any{"rule": rule, "summary": sum})
}

func (s *Server) ruleUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.tv.HasTuner() {
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
	if !currentUser(r.Context()).mayManage(cur.OwnerID) {
		writeErr(w, forbidden("only whoever made this rule, or an admin, can change it"))
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
	if !s.tv.HasTuner() {
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
	if !currentUser(r.Context()).mayManage(cur.OwnerID) {
		writeErr(w, forbidden("only whoever made this rule, or an admin, can delete it"))
		return
	}
	if err := s.tv.DeleteRule(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
