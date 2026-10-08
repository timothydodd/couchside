package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/timothydodd/couchside/internal/auth"
)

// The first run (Windows and plain executables, or any new server): the web
// app asks for your name, an optional password and your media locations,
// then marks setup done. Servers already in use were marked by migration
// 0035, so upgrading skips it.

const settingSetupComplete = "setup.complete"

func (s *Server) firstRun(ctx context.Context) (bool, error) {
	v, err := s.db.Setting(ctx, settingSetupComplete)
	return v != "1", err
}

// welcome is the first-run "who are you": it names the server's admin
// profile, gives it a password if one is typed, and signs this browser in.
// Only while setup isn't done and sign-in is passwordless, when anyone who
// can reach the server could pick that profile anyway; never for a profile
// that already has a password.
func (s *Server) welcome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	first, err := s.firstRun(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	on, _, err := s.passwordless(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !first || !on {
		writeErr(w, forbidden("this server is already set up; sign in instead"))
		return
	}
	if !sameOrigin(r) {
		writeErr(w, forbidden("cross-site request refused"))
		return
	}
	ip := clientIP(r)
	if wait := s.auth.picks.Wait(ip); wait > 0 {
		retryLater(w, wait)
		return
	}
	s.auth.picks.Fail(ip)
	var in struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	pi := profileInput{Name: strings.TrimSpace(in.Name)}
	if err := pi.validate(); err != nil {
		writeErr(w, err)
		return
	}
	profiles, err := s.db.Profiles(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	admin := -1
	for i, p := range profiles {
		if p.Role == "admin" && !p.Disabled && (admin < 0 || p.ID < profiles[admin].ID) {
			admin = i
		}
	}
	if admin < 0 {
		writeErr(w, badRequest("there's no admin profile to set up"))
		return
	}
	p := profiles[admin]
	if p.HasPassword {
		writeErr(w, forbidden(p.Name+" already has a password; sign in instead"))
		return
	}
	if in.Password != "" {
		if err := auth.CheckPassword(in.Password); err != nil {
			writeErr(w, badRequest(err.Error()))
			return
		}
	}
	if p, err = s.db.UpdateProfile(ctx, p.ID, pi.Name, p.Color); err != nil {
		writeErr(w, profileErr(err))
		return
	}
	if in.Password != "" {
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := s.db.SetPassword(ctx, p.ID, hash, false); err != nil {
			writeErr(w, err)
			return
		}
		p.HasPassword = true
	}
	slog.Info("first run: admin named", "profile", p.Name, "password", in.Password != "", "ip", ip)
	s.startSession(w, r, p, "web", "")
}

// finishSetup marks the first run done (admins: the media step is theirs).
func (s *Server) finishSetup(w http.ResponseWriter, r *http.Request) {
	if err := s.db.SetSetting(r.Context(), settingSetupComplete, "1"); err != nil {
		writeErr(w, err)
		return
	}
	slog.Info("first run: setup finished")
	w.WriteHeader(http.StatusNoContent)
}
