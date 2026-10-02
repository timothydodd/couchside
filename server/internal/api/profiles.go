package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/timothydodd/couchside/internal/db"
)

// profileCookie names the browser's active profile: which of its refresh
// cookies (one per signed-in profile) /api/auth/refresh uses.
const profileCookie = "couchside_profile"

// profileColors are the avatar colours the UI offers (token names).
var profileColors = map[string]bool{"accent": true, "pink": true, "cyan": true, "good": true, "warning": true, "critical": true, "secondary": true}

// listProfiles returns the signed-in profile: a profile is an account, and
// the others are managed in Settings → Accounts.
func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	p, err := s.db.Profile(r.Context(), currentUser(r.Context()).ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": []db.Profile{p}, "current": p.ID, "chosen": true})
}

type profileInput struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (in *profileInput) validate() error {
	in.Name = strings.Join(strings.Fields(in.Name), " ")
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 30 {
		return badRequest("a profile name needs 1 to 30 characters")
	}
	if in.Color == "" {
		in.Color = "accent"
	}
	if !profileColors[in.Color] {
		return badRequest("unknown colour")
	}
	return nil
}

func profileErr(err error) error {
	if errors.Is(err, db.ErrProfileName) || errors.Is(err, db.ErrLastProfile) || errors.Is(err, db.ErrPrefsNotAnObj) {
		return badRequest(err.Error())
	}
	return err
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !currentUser(r.Context()).mayManage(id) {
		writeErr(w, db.ErrNotFound)
		return
	}
	var in profileInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.db.UpdateProfile(r.Context(), id, in.Name, in.Color)
	if err != nil {
		writeErr(w, profileErr(err))
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// profilePrefs merges top-level keys into a profile's preferences.
func (s *Server) profilePrefs(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if id != currentUser(r.Context()).ID {
		writeErr(w, db.ErrNotFound)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil || !json.Valid(body) {
		writeErr(w, badRequest("bad JSON body"))
		return
	}
	p, err := s.db.MergeProfilePrefs(r.Context(), id, body)
	if err != nil {
		writeErr(w, profileErr(err))
		return
	}
	writeJSON(w, http.StatusOK, p)
}
