package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/timothydodd/couchside/internal/db"
)

// Passwordless sign-in. Accounts are always on, but a home server needn't make
// people type passwords: with passwordless on, the sign-in screen lists the
// profiles and picking one without a password signs in (with real tokens,
// exactly like a password sign-in). Profiles that have a password still ask
// for it, so an admin can lock theirs. COUCHSIDE_AUTH=true turns passwordless
// off for good (for servers on the internet).
const settingPasswordless = "auth.passwordless"

// passwordless says whether picking a profile signs in, and whether the
// setting is locked off by COUCHSIDE_AUTH. The first time, it's on unless a
// profile already has a password (an install that used passwords stays shut),
// and that choice is saved.
func (s *Server) passwordless(ctx context.Context) (on, locked bool, err error) {
	if s.cfg.Auth {
		return false, true, nil
	}
	v, err := s.db.Setting(ctx, settingPasswordless)
	if err != nil {
		return false, false, err
	}
	if v != "" {
		return v == "1", false, nil
	}
	// First time asked: decide once and keep it, so setting a password later
	// doesn't silently switch passwordless off for everyone.
	any, err := s.db.AnyPassword(ctx)
	if err != nil {
		return false, false, err
	}
	v = "1"
	if any {
		v = "0"
	}
	return !any, false, s.db.SetSetting(ctx, settingPasswordless, v)
}

// setPasswordless turns passwordless sign-in on or off (admins).
func (s *Server) setPasswordless(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	if _, locked, err := s.passwordless(ctx); err != nil {
		writeErr(w, err)
		return
	} else if locked {
		writeErr(w, badRequest("COUCHSIDE_AUTH=true on the server requires passwords"))
		return
	}
	if !in.Enabled {
		// Without passwordless, someone must still be able to sign in and manage the server.
		ok, err := s.db.HasAdmin(ctx)
		if err != nil {
			writeErr(w, err)
			return
		}
		if !ok {
			writeErr(w, badRequest("set a password on an admin account first, or nobody could sign in to manage the server"))
			return
		}
	}
	v := "0"
	if in.Enabled {
		v = "1"
	}
	if err := s.db.SetSetting(ctx, settingPasswordless, v); err != nil {
		writeErr(w, err)
		return
	}
	if !in.Enabled {
		if err := s.endPasswordlessSessions(ctx); err != nil {
			writeErr(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// endPasswordlessSessions signs out profiles without a password, which can't
// sign in once passwordless sign-in is off.
func (s *Server) endPasswordlessSessions(ctx context.Context) error {
	n, err := s.db.DeleteSessionsWithoutPassword(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Info("passwordless sign-in is off: signed out profiles without a password", "sessions", n)
	}
	s.auth.sessions.forgetAll()
	return nil
}

type pickInput struct {
	ProfileID int64  `json:"profileId"`
	Client    string `json:"client"` // web (default) | tv
	Device    string `json:"device"`
}

// pick signs in to a profile without a password, when passwordless is on.
func (s *Server) pick(w http.ResponseWriter, r *http.Request) {
	var in pickInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Client == "" {
		in.Client = "web"
	}
	if in.Client != "web" && in.Client != "tv" {
		writeErr(w, badRequest("client must be web or tv"))
		return
	}
	ctx := r.Context()
	on, _, err := s.passwordless(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !on {
		writeErr(w, forbidden("this server requires a password; sign in with your name and password"))
		return
	}
	if in.Client == "web" && !sameOrigin(r) {
		writeErr(w, forbidden("cross-site request refused"))
		return
	}
	ip := clientIP(r)
	if first, err := s.firstRun(ctx); err != nil {
		writeErr(w, err)
		return
	} else if first && !localClient(r) {
		refuseRemoteFirstRun(w, ip)
		return
	}
	if wait := s.auth.picks.Wait(ip); wait > 0 {
		retryLater(w, wait)
		return
	}
	p, err := s.db.Profile(ctx, in.ProfileID)
	if errors.Is(err, db.ErrNotFound) {
		writeErr(w, db.ErrNotFound)
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	if p.Disabled {
		writeErr(w, forbidden("this account is disabled"))
		return
	}
	if p.HasPassword {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": p.Name + " has a password", "code": "password_required"})
		return
	}
	s.auth.picks.Fail(ip) // every pick counts: each one makes a session
	s.startSession(w, r, p, in.Client, in.Device)
}

// Hiding admins: with this on, the passwordless picker leaves admin accounts
// out, and they're listed at /admin instead (GET /api/auth?admin=1). It only
// keeps them out of sight (a demo's reviewers needn't see the admin account);
// what protects an admin account is its password.
const settingHideAdmins = "auth.hide_admins"

func (s *Server) hideAdmins(ctx context.Context) (bool, error) {
	v, err := s.db.Setting(ctx, settingHideAdmins)
	return v == "1", err
}

// setHideAdmins hides admin accounts from the profile picker, or shows them (admins).
func (s *Server) setHideAdmins(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v := "0"
	if in.Enabled {
		v = "1"
	}
	if err := s.db.SetSetting(r.Context(), settingHideAdmins, v); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pickable lists the enabled profiles for the passwordless picker. While
// admins are hidden it's everyone else, or only the admins for the /admin
// page (admins true).
func (s *Server) pickable(ctx context.Context, admins bool) ([]profileStub, error) {
	hide, err := s.hideAdmins(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := s.db.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	out := []profileStub{}
	for _, p := range ps {
		if p.Disabled || (hide && (p.Role == "admin") != admins) {
			continue
		}
		out = append(out, profileStub{ID: p.ID, Name: p.Name, Color: p.Color, HasPassword: p.HasPassword})
	}
	return out, nil
}
