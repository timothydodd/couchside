package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/timothydodd/couchside/internal/db"
)

// profileCookie holds the chosen profile id. Without it (or with a stale id)
// requests act as the oldest profile, so a single-profile install needs no picker.
const profileCookie = "couchside_profile"

// profileColors are the avatar colours the UI offers (token names).
var profileColors = map[string]bool{"accent": true, "pink": true, "cyan": true, "good": true, "warning": true, "critical": true, "secondary": true}

// profileIDs caches which profile ids exist, since every API request resolves one.
type profileIDs struct {
	mu    sync.Mutex
	ids   map[int64]bool
	first int64
}

func (c *profileIDs) load(ctx context.Context, d *db.DB) error {
	ps, err := d.Profiles(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ids = map[int64]bool{}
	c.first = 0
	for _, p := range ps {
		c.ids[p.ID] = true
		if c.first == 0 {
			c.first = p.ID
		}
	}
	return nil
}

// resolveProfile returns the request's profile, and whether it was chosen by cookie.
func (s *Server) resolveProfile(r *http.Request) (int64, bool) {
	c := &s.profiles
	c.mu.Lock()
	loaded := c.ids != nil
	c.mu.Unlock()
	if !loaded {
		if err := c.load(r.Context(), s.db); err != nil {
			slog.Error("load profiles", "err", err)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ck, err := r.Cookie(profileCookie); err == nil {
		if id, err := strconv.ParseInt(ck.Value, 10, 64); err == nil && c.ids[id] {
			return id, true
		}
	}
	return c.first, false
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth {
		// Signed in, you see yourself; other accounts are in the account manager.
		u := currentUser(r.Context())
		p, err := s.db.Profile(r.Context(), u.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"profiles": []db.Profile{p}, "current": p.ID, "chosen": true})
		return
	}
	ps, err := s.db.Profiles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	id, chosen := s.resolveProfile(r)
	writeJSON(w, http.StatusOK, map[string]any{"profiles": ps, "current": id, "chosen": chosen})
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

// errUseAccounts refuses the picker's profile management when accounts are on.
var errUseAccounts = badRequest("accounts are on: add and remove people in Settings → Accounts")

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth {
		writeErr(w, errUseAccounts)
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
	p, err := s.db.CreateProfile(r.Context(), in.Name, in.Color)
	if err != nil {
		writeErr(w, profileErr(err))
		return
	}
	_ = s.profiles.load(r.Context(), s.db)
	writeJSON(w, http.StatusCreated, p)
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
	if s.cfg.Auth && id != currentUser(r.Context()).ID {
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

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth {
		writeErr(w, errUseAccounts)
		return
	}
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.DeleteProfile(r.Context(), id); err != nil {
		writeErr(w, profileErr(err))
		return
	}
	_ = s.profiles.load(r.Context(), s.db)
	w.WriteHeader(http.StatusNoContent)
}

// selectProfile remembers the chosen profile in this browser for a year.
func (s *Server) selectProfile(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth {
		writeErr(w, badRequest("accounts are on: sign in to switch profiles"))
		return
	}
	id, err := idParam(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.db.Profile(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: profileCookie, Value: strconv.FormatInt(id, 10), Path: "/",
		MaxAge: 365 * 24 * 3600, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}
