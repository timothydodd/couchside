package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/timothydodd/couchside/internal/auth"
)

type passwordInput struct {
	Current  string `json:"current"`
	Password string `json:"password"`
}

// changePassword sets the user's own password and signs out their other devices.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := currentUser(ctx)
	var in passwordInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.db.Profile(ctx, u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if p.PasswordLocked && p.Role != "admin" {
		writeErr(w, httpError{http.StatusForbidden, "only an admin can change this account's password"})
		return
	}
	ip := clientIP(r)
	nameKey := strings.ToLower(p.Name)
	done, ok := s.attempt(w, ip, nameKey)
	if !ok {
		return
	}
	hash, err := s.db.PasswordHash(ctx, u.ID)
	if err != nil {
		done(false)
		writeErr(w, err)
		return
	}
	// A profile without a password (passwordless sign-in) sets its first one.
	ok, _ = auth.VerifyPassword(hash, in.Current)
	wrong := hash != "" && !ok
	done(wrong)
	if wrong {
		slog.Warn("password change: wrong current password", "ip", ip, "name", nameKey)
		writeErr(w, badRequest("your current password isn't right"))
		return
	}
	if err := auth.CheckPassword(in.Password); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	if hash != "" && in.Password == in.Current {
		writeErr(w, badRequest("choose a different password"))
		return
	}
	newHash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetPassword(ctx, u.ID, newHash, false); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.DeleteSessions(ctx, u.ID, u.Session); err != nil {
		writeErr(w, err)
		return
	}
	s.auth.sessions.forgetAll()
	slog.Info("password changed", "profile", p.Name, "ip", ip)
	w.WriteHeader(http.StatusNoContent)
}
