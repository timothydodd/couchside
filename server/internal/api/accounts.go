package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

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
	out := make([]accountView, len(ps))
	for i, p := range ps {
		out[i] = s.account(r.Context(), p)
	}
	writeJSON(w, http.StatusOK, out)
}

// accountView is a profile as the account manager shows it: with its limits.
type accountView struct {
	db.Profile
	Libraries []int64 `json:"libraries"` // empty = every library
	MaxRating string  `json:"maxRating"` // "" = any rating
}

func (s *Server) account(ctx context.Context, p db.Profile) accountView {
	v := accountView{Profile: p, Libraries: []int64{}}
	if a, err := s.db.ProfileAccess(ctx, p.ID); err == nil {
		v.MaxRating = a.MaxRating
		if a.Libraries != nil {
			v.Libraries = a.Libraries
		}
	}
	return v
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
	// Libraries limits the account to these libraries (empty = all), and
	// MaxRating to titles rated no higher ("" = any). Neither applies to admins.
	Libraries []int64 `json:"libraries"`
	MaxRating string  `json:"maxRating"`
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
		in.Libraries, in.MaxRating = nil, ""
	}
	if in.MaxRating != "" && !slices.Contains(db.RatingLimits, in.MaxRating) {
		return badRequest("the rating limit must be one of " + strings.Join(db.RatingLimits, ", "))
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
	p, err := s.db.CreateAccount(r.Context(), in.Name, in.Color, in.Role, in.CanRecord, hash, in.PasswordLocked)
	if err != nil {
		writeErr(w, accountErr(err))
		return
	}
	if err := s.db.SetProfileAccess(r.Context(), p.ID, db.Access{Libraries: in.Libraries, MaxRating: in.MaxRating}); err != nil {
		writeErr(w, accountErr(err))
		return
	}
	slog.Info("account created", "profile", p.Name, "role", p.Role, "by", currentUser(r.Context()).ID)
	writeJSON(w, http.StatusCreated, s.account(r.Context(), p))
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
	// The new role and status are committed: whatever fails below (a taken
	// name, say), cached sessions mustn't keep the old ones, and a disabled
	// account is signed out now.
	defer s.auth.sessions.forgetAll()
	if in.Disabled {
		if err := s.db.DeleteSessions(ctx, id, ""); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err := s.db.SetPasswordLocked(ctx, id, in.PasswordLocked); err != nil {
		writeErr(w, accountErr(err))
		return
	}
	if err := s.db.SetProfileAccess(ctx, id, db.Access{Libraries: in.Libraries, MaxRating: in.MaxRating}); err != nil {
		writeErr(w, accountErr(err))
		return
	}
	p, err := s.db.UpdateProfile(ctx, id, in.Name, in.Color)
	if err != nil {
		writeErr(w, accountErr(err))
		return
	}
	writeJSON(w, http.StatusOK, s.account(ctx, p))
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
