package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/timothydodd/couchside/internal/auth"
	"github.com/timothydodd/couchside/internal/db"
)

// The account manager (admins only, Settings → Accounts). Accounts are
// profiles: these add the password, role, recording permission and status.

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	ps, err := s.db.Profiles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

type accountInput struct {
	profileInput
	Role      string `json:"role"`
	CanRecord bool   `json:"canRecord"`
	Disabled  bool   `json:"disabled"`
	Password  string `json:"password"` // create only: a temporary password
	// PasswordLocked stops the account setting or changing its own password
	// (a shared profile). Admins can always change theirs.
	PasswordLocked bool `json:"passwordLocked"`
}

func (in *accountInput) validate() error {
	if err := in.profileInput.validate(); err != nil {
		return err
	}
	if in.Role == "" {
		in.Role = "user"
	}
	if in.Role != "admin" && in.Role != "user" {
		return badRequest("role must be admin or user")
	}
	if in.Role == "admin" {
		in.PasswordLocked = false
	}
	return nil
}

func accountErr(err error) error {
	if errors.Is(err, db.ErrLastAdmin) {
		return badRequest(err.Error())
	}
	return profileErr(err)
}

// createAccount adds a profile with a temporary password, to be replaced at
// its first sign-in.
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var in accountInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	// With passwordless sign-in a password is optional: none means "pick me".
	hash := ""
	on, _, err := s.passwordless(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if in.Password != "" || !on {
		if err := auth.CheckPassword(in.Password); err != nil {
			writeErr(w, badRequest(err.Error()))
			return
		}
		if hash, err = auth.HashPassword(in.Password); err != nil {
			writeErr(w, err)
			return
		}
	}
	// An admin's temporary password for a locked account is the password: it
	// can't be replaced at first sign-in.
	p, err := s.db.CreateAccount(r.Context(), in.Name, in.Color, in.Role, in.CanRecord, hash)
	if err != nil {
		writeErr(w, accountErr(err))
		return
	}
	if in.PasswordLocked {
		if err := s.db.SetPasswordLocked(r.Context(), p.ID, true); err == nil && hash != "" {
			err = s.db.SetPassword(r.Context(), p.ID, hash, false)
		}
		if p, err = s.db.Profile(r.Context(), p.ID); err != nil {
			writeErr(w, err)
			return
		}
	}
	slog.Info("account created", "profile", p.Name, "role", p.Role, "by", currentUser(r.Context()).ID)
	writeJSON(w, http.StatusCreated, p)
}

// updateAccount renames and recolours a profile and sets its access.
// Disabling one signs it out everywhere.
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in accountInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	if _, err := s.db.SetAccess(ctx, id, in.Role, in.CanRecord, in.Disabled); err != nil {
		writeErr(w, accountErr(err))
		return
	}
	if err := s.db.SetPasswordLocked(ctx, id, in.PasswordLocked); err != nil {
		writeErr(w, accountErr(err))
		return
	}
	p, err := s.db.UpdateProfile(ctx, id, in.Name, in.Color)
	if err != nil {
		writeErr(w, accountErr(err))
		return
	}
	if in.Disabled {
		if err := s.db.DeleteSessions(ctx, id, ""); err != nil {
			writeErr(w, err)
			return
		}
	}
	s.auth.sessions.forgetAll()
	writeJSON(w, http.StatusOK, p)
}

type resetInput struct {
	Password string `json:"password"`
}

// resetPassword gives an account a temporary password and signs it out everywhere.
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in resetInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	hash, mustChange := "", false
	if in.Password == "" {
		// Removing a password only makes sense when profiles can be picked without one.
		on, _, err := s.passwordless(ctx)
		if err != nil {
			writeErr(w, err)
			return
		}
		if !on {
			writeErr(w, badRequest("passwordless sign-in is off, so every account needs a password"))
			return
		}
	} else {
		if err := auth.CheckPassword(in.Password); err != nil {
			writeErr(w, badRequest(err.Error()))
			return
		}
		if hash, err = auth.HashPassword(in.Password); err != nil {
			writeErr(w, err)
			return
		}
		// A temporary password, unless the account can't change it.
		p, err := s.db.Profile(ctx, id)
		if err != nil {
			writeErr(w, err)
			return
		}
		mustChange = !p.PasswordLocked
	}
	if err := s.db.SetPassword(ctx, id, hash, mustChange); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.DeleteSessions(ctx, id, ""); err != nil {
		writeErr(w, err)
		return
	}
	s.auth.sessions.forgetAll()
	slog.Info("password reset by admin", "profile", id, "by", currentUser(ctx).ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if id == currentUser(r.Context()).ID {
		writeErr(w, badRequest("you can't delete your own account"))
		return
	}
	if err := s.db.DeleteProfile(r.Context(), id); err != nil {
		writeErr(w, accountErr(err))
		return
	}
	s.auth.sessions.forgetAll()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) accountSessions(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.listSessions(w, r, id)
}
