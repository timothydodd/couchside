package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/db"
)

type sessionView struct {
	db.Session
	Current bool `json:"current"`
}

// mySessions lists the user's signed-in devices.
func (s *Server) mySessions(w http.ResponseWriter, r *http.Request) {
	s.listSessions(w, r, currentUser(r.Context()).ID)
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request, profileID int64) {
	ss, err := s.db.Sessions(r.Context(), profileID, time.Now().Unix())
	if err != nil {
		writeErr(w, err)
		return
	}
	cur := currentUser(r.Context()).Session
	out := make([]sessionView, 0, len(ss))
	for _, x := range ss {
		out = append(out, sessionView{x, x.ID == cur})
	}
	writeJSON(w, http.StatusOK, out)
}

// endSession signs one device out: your own, or anyone's for an admin.
func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := currentUser(ctx)
	sess, err := s.db.Session(ctx, chi.URLParam(r, "sid"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if !u.mayManage(sess.ProfileID) {
		writeErr(w, db.ErrNotFound)
		return
	}
	if err := s.db.DeleteSession(ctx, sess.ID); err != nil {
		writeErr(w, err)
		return
	}
	s.auth.sessions.forget(sess.ID)
	w.WriteHeader(http.StatusNoContent)
}

// endOtherSessions signs the user out everywhere but here.
func (s *Server) endOtherSessions(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if err := s.db.DeleteSessions(r.Context(), u.ID, u.Session); err != nil {
		writeErr(w, err)
		return
	}
	s.auth.sessions.forgetAll()
	w.WriteHeader(http.StatusNoContent)
}

// pruneSessions drops expired sessions now and then.
func (s *Server) pruneSessions(ctx context.Context) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		if err := s.db.PruneSessions(ctx, time.Now().Unix()); err != nil && ctx.Err() == nil {
			slog.Warn("prune sessions", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
