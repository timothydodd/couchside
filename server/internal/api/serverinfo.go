package api

import (
	"context"
	"net/http"
	"sort"
)

// APIVersion goes up only when an existing route or field changes meaning or
// goes away. Adding routes, fields and feature names never changes it.
// docs/api-compat.md has the rules; clients compare it with the oldest
// version they understand.
const APIVersion = 1

// serverInfo is open, like discovery: what a client needs before signing in
// to decide whether it can talk to this server at all.
func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"app":        "couchside",
		"id":         s.cfg.ServerID,
		"name":       s.cfg.ServerName,
		"version":    s.version,
		"apiVersion": APIVersion,
		"signIn":     s.SignInMode(r.Context()),
		"features":   s.features(r.Context()),
	})
}

// features names what this server can do right now, from its real state
// rather than its settings, sorted. A client treats a name it doesn't know
// as nothing, and a server without /api/server (older than 0.19) as having
// every surface it had then.
func (s *Server) features(ctx context.Context) []string {
	f := []string{
		// API surfaces every server since 0.19 has.
		"clientLog", "deviceCode", "editions", "hls", "optimize", "people", "segments", "totp", "trickplay", "virtualChannels", "watchlist",
	}
	if s.tc != nil && s.tc.Encoder().HW != "" && s.tc.Encoder().HW != "none" {
		f = append(f, "hwaccel")
	}
	if s.tv != nil {
		if sum := s.tv.Summary(); sum["configured"] == true {
			f = append(f, "livetv")
		}
		if s.tv.HasTuner() {
			f = append(f, "tuner", "dvr")
		}
	}
	if s.worker != nil && s.worker.CommercialsAvailable() {
		f = append(f, "commercials")
	}
	if !s.providers.Empty() {
		f = append(f, "metadata")
		if len(s.providers.Degraded()) > 0 {
			f = append(f, "metadataDegraded")
		}
	}
	if s.db != nil {
		if on, _, err := s.passwordless(ctx); err == nil && on {
			f = append(f, "passwordless")
		}
		if s.oidcConfig(ctx).enabled() {
			f = append(f, "oidc")
		}
	}
	if s.cfg.Discovery {
		f = append(f, "discovery")
	}
	sort.Strings(f)
	return f
}
