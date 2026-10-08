package api

import (
	"log/slog"
	"maps"
	"net/http"
	"strings"

	"github.com/timothydodd/couchside/internal/netshare"
)

// Settings → Server → Network shares (Windows only): sign-ins for NAS
// shares the service's own account can't read. Saving signs in at once, so
// the media folder and libraries can be picked from the share straight
// away; start-up signs in again (main, before anything reads the media).

// UseShares hands over how each share's sign-in went at start-up.
func (s *Server) UseShares(status map[string]string) {
	s.sharesMu.Lock()
	s.shareStatus = maps.Clone(status)
	s.sharesMu.Unlock()
}

type shareRow struct {
	Path        string `json:"path"`
	User        string `json:"user"`
	HasPassword bool   `json:"hasPassword"`
	Error       string `json:"error"` // "" when signed in
}

func (s *Server) listShares(w http.ResponseWriter, r *http.Request) {
	if !netshare.Supported {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "shares": []shareRow{}})
		return
	}
	shares, err := s.db.NetShares(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	s.sharesMu.Lock()
	defer s.sharesMu.Unlock()
	out := make([]shareRow, 0, len(shares))
	for _, sh := range shares {
		e, ok := s.shareStatus[sh.Path]
		if !ok {
			e = "not signed in yet"
		}
		out = append(out, shareRow{Path: sh.Path, User: sh.User, HasPassword: len(sh.Secret) > 0, Error: e})
	}
	writeJSON(w, http.StatusOK, map[string]any{"supported": true, "shares": out})
}

// setShares replaces the list. A share sent without a password keeps the
// one saved for it. Each is signed in to now; one that fails is still saved
// (the NAS may just be off), with its error shown.
func (s *Server) setShares(w http.ResponseWriter, r *http.Request) {
	if !netshare.Supported {
		writeErr(w, badRequest(netshare.ErrUnsupported.Error()))
		return
	}
	var in []struct {
		Path     string  `json:"path"`
		User     string  `json:"user"`
		Password *string `json:"password"` // nil keeps the saved one
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	old, err := s.db.NetShares(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	saved := map[string]netshare.Share{}
	for _, sh := range old {
		saved[strings.ToLower(sh.Path)] = sh
	}
	var next []netshare.Share
	passwords := map[string]string{}
	seen := map[string]bool{}
	for _, x := range in {
		root, err := netshare.Root(x.Path)
		if err != nil {
			writeErr(w, badRequest(err.Error()))
			return
		}
		key := strings.ToLower(root)
		if seen[key] {
			writeErr(w, badRequest(root+" is listed twice"))
			return
		}
		seen[key] = true
		sh := netshare.Share{Path: root, User: strings.TrimSpace(x.User)}
		if x.Password != nil {
			if sh.Secret, err = netshare.Protect(*x.Password); err != nil {
				writeErr(w, err)
				return
			}
			passwords[root] = *x.Password
		} else if prev, ok := saved[key]; ok {
			sh.Secret = prev.Secret
		}
		next = append(next, sh)
	}
	if err := s.db.SetNetShares(r.Context(), next); err != nil {
		writeErr(w, err)
		return
	}
	// Removed shares are signed out; the rest are signed in with what's saved now.
	for key, sh := range saved {
		if !seen[key] {
			_ = netshare.Disconnect(sh.Path)
		}
	}
	status := netshare.ConnectAll(next)
	for p, e := range status {
		if e != "" {
			slog.Warn("network share sign-in failed", "share", p, "err", e)
		} else {
			slog.Info("network share signed in", "share", p)
		}
	}
	s.UseShares(status)
	s.listShares(w, r)
}
