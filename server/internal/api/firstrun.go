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

// msgFirstRunRemote is why an unclaimed server refuses a visitor from the
// internet: until setup is done, anyone who can reach it could claim it.
const msgFirstRunRemote = "Couchside can only be set up from your home network. Open it on a device on the same network as the server, or enter the setup code from the server log."

func refuseRemoteFirstRun(w http.ResponseWriter, ip string) {
	slog.Warn("first run: refused a visitor from the internet", "ip", ip)
	writeJSON(w, http.StatusForbidden, map[string]string{"error": msgFirstRunRemote, "code": "private_only"})
}

// firstRunCode makes and logs, once, the code that lets the owner finish the
// first run from outside the home network. It shares the setup code with
// setupNeeded (used when passwords are required).
func (s *Server) firstRunCode() {
	s.auth.mu.Lock()
	defer s.auth.mu.Unlock()
	if s.auth.setupCode == "" {
		s.auth.setupCode = auth.SetupCode()
		slog.Warn("Couchside isn't set up yet: open it in a browser on your home network, or from anywhere else enter this setup code",
			"code", s.auth.setupCode)
	}
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
		Code     string `json:"code"` // the setup code, needed from outside the home network
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	remote := !localClient(r)
	if remote {
		if strings.TrimSpace(in.Code) == "" {
			refuseRemoteFirstRun(w, ip)
			return
		}
		done, ok := s.attempt(w, ip, "setup")
		if !ok {
			return
		}
		s.auth.mu.Lock()
		code := s.auth.setupCode
		s.auth.mu.Unlock()
		wrong := code == "" || !auth.SameCode(in.Code, code)
		done(wrong)
		if wrong {
			slog.Warn("wrong setup code", "ip", ip)
			writeErr(w, httpError{http.StatusUnauthorized, "that setup code isn't right; it's in the server log"})
			return
		}
		if in.Password == "" {
			writeErr(w, badRequest("choose a password: this server can be reached from the internet"))
			return
		}
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
	s.auth.mu.Lock()
	s.auth.setupCode = "" // claimed: the code is spent
	s.auth.mu.Unlock()
	slog.Info("first run: admin named", "profile", p.Name, "password", in.Password != "", "remote", remote, "ip", ip)
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
