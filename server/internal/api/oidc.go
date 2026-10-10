package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/timothydodd/couchside/internal/auth"
	"github.com/timothydodd/couchside/internal/db"
)

// Sign-in through an OpenID Connect provider (Authelia, Authentik, Keycloak,
// Google…): the authorization-code flow with PKCE, for the web. The provider
// says who someone is; the claim chosen in Settings is matched to a profile
// by name, and the session that follows is an ordinary one. Local passwords
// keep working, and `couchside reset-password` is still the way back in.
//
// The ID token is read from the token endpoint's answer, which Couchside
// fetches itself over TLS with its client secret, so (as the spec allows for
// that case) its signature isn't checked; issuer, audience, expiry and nonce
// are.

const (
	settingOIDCIssuer   = "oidc.issuer"
	settingOIDCClient   = "oidc.client_id"
	settingOIDCSecret   = "oidc.client_secret"
	settingOIDCClaim    = "oidc.claim"       // which claim names the profile; default preferred_username
	settingOIDCCreate   = "oidc.create"      // "1": make a profile for someone new
	settingOIDCAdmins   = "oidc.admin_group" // a value in the "groups" claim that makes an admin
	settingOIDCLabel    = "oidc.label"       // the button's text; default "Sign in with single sign-on"
	oidcStateFor        = 10 * time.Minute
	oidcMaxPending      = 1000
	defaultOIDCClaim    = "preferred_username"
	defaultOIDCLabel    = "Sign in with single sign-on"
	oidcDiscoveryKeep   = time.Hour
	oidcResponseMaxSize = 1 << 20
)

type oidcConfig struct {
	Issuer     string `json:"issuer"`
	ClientID   string `json:"clientId"`
	Secret     string `json:"clientSecret,omitempty"` // write-only: never sent back
	HasSecret  bool   `json:"hasSecret"`
	Claim      string `json:"claim"`
	Create     bool   `json:"create"`
	AdminGroup string `json:"adminGroup"`
	Label      string `json:"label"`
}

func (c oidcConfig) enabled() bool { return c.Issuer != "" && c.ClientID != "" }

func (s *Server) oidcConfig(ctx context.Context) oidcConfig {
	get := func(k string) string { v, _ := s.db.Setting(ctx, k); return v }
	c := oidcConfig{Issuer: strings.TrimRight(get(settingOIDCIssuer), "/"), ClientID: get(settingOIDCClient), Secret: get(settingOIDCSecret),
		Claim: get(settingOIDCClaim), Create: get(settingOIDCCreate) == "1", AdminGroup: get(settingOIDCAdmins), Label: get(settingOIDCLabel)}
	c.HasSecret = c.Secret != ""
	if c.Claim == "" {
		c.Claim = defaultOIDCClaim
	}
	if c.Label == "" {
		c.Label = defaultOIDCLabel
	}
	return c
}

// oidcState is the server's memory of sign-ins in progress and of each
// provider's endpoints.
type oidcState struct {
	mu        sync.Mutex
	pending   map[string]oidcPending // by state
	discovery map[string]oidcEndpoints
}

type oidcPending struct {
	nonce, verifier, redirect string
	expires                   time.Time
}

type oidcEndpoints struct {
	Issuer string `json:"issuer"`
	Auth   string `json:"authorization_endpoint"`
	Token  string `json:"token_endpoint"`
	at     time.Time
}

var oidcClient = &http.Client{Timeout: 15 * time.Second}

// endpoints fetches (and keeps for an hour) the provider's discovery document.
func (s *Server) oidcEndpoints(ctx context.Context, issuer string) (oidcEndpoints, error) {
	s.oidc.mu.Lock()
	e, ok := s.oidc.discovery[issuer]
	s.oidc.mu.Unlock()
	if ok && time.Since(e.at) < oidcDiscoveryKeep {
		return e, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return e, err
	}
	res, err := oidcClient.Do(req)
	if err != nil {
		return e, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return e, fmt.Errorf("discovery: HTTP %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, oidcResponseMaxSize)).Decode(&e); err != nil {
		return e, fmt.Errorf("discovery: %w", err)
	}
	if e.Auth == "" || e.Token == "" {
		return e, errors.New("discovery: the provider's document has no authorization or token endpoint")
	}
	if strings.TrimRight(e.Issuer, "/") != issuer {
		return e, fmt.Errorf("discovery: the document is for %q, not %q", e.Issuer, issuer)
	}
	e.at = time.Now()
	s.oidc.mu.Lock()
	if s.oidc.discovery == nil {
		s.oidc.discovery = map[string]oidcEndpoints{}
	}
	s.oidc.discovery[issuer] = e
	s.oidc.mu.Unlock()
	return e, nil
}

// origin is this server's address as the browser sees it.
func origin(r *http.Request) string {
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// oidcStart sends the browser to the provider.
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	cfg := s.oidcConfig(r.Context())
	if !cfg.enabled() {
		http.NotFound(w, r)
		return
	}
	ep, err := s.oidcEndpoints(r.Context(), cfg.Issuer)
	if err != nil {
		slog.Error("oidc", "err", err)
		oidcFailed(w, r, "The sign-in provider couldn't be reached. The server log has the reason.")
		return
	}
	state, p := auth.NewToken(), oidcPending{nonce: auth.NewToken(), verifier: auth.NewToken(),
		redirect: origin(r) + "/api/auth/oidc/callback", expires: time.Now().Add(oidcStateFor)}
	s.oidc.mu.Lock()
	if s.oidc.pending == nil {
		s.oidc.pending = map[string]oidcPending{}
	}
	for k, v := range s.oidc.pending {
		if time.Now().After(v.expires) {
			delete(s.oidc.pending, k)
		}
	}
	full := len(s.oidc.pending) >= oidcMaxPending
	if !full {
		s.oidc.pending[state] = p
	}
	s.oidc.mu.Unlock()
	if full {
		oidcFailed(w, r, "Too many sign-ins are in progress. Try again in a few minutes.")
		return
	}
	challenge := sha256.Sum256([]byte(p.verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {cfg.ClientID}, "redirect_uri": {p.redirect},
		"scope": {"openid profile email"}, "state": {state}, "nonce": {p.nonce},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
	sep := "?"
	if strings.Contains(ep.Auth, "?") {
		sep = "&"
	}
	http.Redirect(w, r, ep.Auth+sep+q.Encode(), http.StatusFound)
}

// oidcFailed sends the browser back to the sign-in page with a message.
func oidcFailed(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/?signin_error="+url.QueryEscape(msg), http.StatusFound)
}

// oidcCallback is where the provider sends the browser back: the code is
// exchanged, the person matched to a profile, and a session started.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := s.oidcConfig(ctx)
	if !cfg.enabled() {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	s.oidc.mu.Lock()
	p, ok := s.oidc.pending[q.Get("state")]
	delete(s.oidc.pending, q.Get("state"))
	s.oidc.mu.Unlock()
	if !ok || time.Now().After(p.expires) {
		oidcFailed(w, r, "That sign-in took too long or was already used. Try again.")
		return
	}
	if e := q.Get("error"); e != "" {
		slog.Warn("oidc: the provider refused", "error", e, "description", clip(q.Get("error_description"), 200))
		oidcFailed(w, r, "The sign-in provider didn't sign you in.")
		return
	}
	claims, err := s.oidcExchange(ctx, cfg, p, q.Get("code"))
	if err != nil {
		slog.Error("oidc: sign-in failed", "err", err, "ip", clientIP(r))
		oidcFailed(w, r, "Sign-in through the provider failed. The server log has the reason.")
		return
	}
	name, _ := claims[cfg.Claim].(string)
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		slog.Error("oidc: the ID token has no usable claim", "claim", cfg.Claim)
		oidcFailed(w, r, "The provider didn't say who you are (no \""+cfg.Claim+"\"). An admin needs to check the claim in Settings.")
		return
	}
	admin := cfg.AdminGroup != "" && slices.Contains(claimList(claims["groups"]), cfg.AdminGroup)
	prof, _, err := s.db.ProfileForLogin(ctx, name)
	switch {
	case errors.Is(err, db.ErrNotFound) && cfg.Create:
		pi := profileInput{Name: name}
		if err := pi.validate(); err != nil {
			oidcFailed(w, r, "Your name at the provider can't be used as a profile name here.")
			return
		}
		role := "user"
		if admin {
			role = "admin"
		}
		// No password, and locked: this account signs in through the provider.
		if prof, err = s.db.CreateAccount(ctx, pi.Name, pi.Color, role, false, "", role != "admin"); err != nil {
			slog.Error("oidc: create profile", "err", err)
			oidcFailed(w, r, "A profile couldn't be made for you. The server log has the reason.")
			return
		}
		slog.Info("account created by single sign-on", "profile", prof.Name, "role", role)
	case errors.Is(err, db.ErrNotFound):
		slog.Warn("oidc: nobody here by that name", "name", name, "ip", clientIP(r))
		oidcFailed(w, r, "There's no Couchside profile called "+name+". Ask an admin to add one.")
		return
	case err != nil:
		slog.Error("oidc", "err", err)
		oidcFailed(w, r, "Sign-in failed. The server log has the reason.")
		return
	}
	if prof.Disabled {
		oidcFailed(w, r, "That account is disabled.")
		return
	}
	if _, err := s.openSession(w, r, prof, "web", "Single sign-on"); err != nil {
		slog.Error("oidc: session", "err", err)
		oidcFailed(w, r, "Sign-in failed. The server log has the reason.")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// oidcExchange trades the code for an ID token and returns its claims, having
// checked they're from this provider, for this client, current, and for this
// sign-in.
func (s *Server) oidcExchange(ctx context.Context, cfg oidcConfig, p oidcPending, code string) (map[string]any, error) {
	if code == "" {
		return nil, errors.New("the provider sent no code")
	}
	ep, err := s.oidcEndpoints(ctx, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {p.redirect},
		"client_id": {cfg.ClientID}, "code_verifier": {p.verifier}}
	if cfg.Secret != "" {
		form.Set("client_secret", cfg.Secret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := oidcClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var tok struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error"`
		Desc    string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, oidcResponseMaxSize)).Decode(&tok); err != nil {
		return nil, fmt.Errorf("token endpoint: HTTP %d: %w", res.StatusCode, err)
	}
	if res.StatusCode != http.StatusOK || tok.IDToken == "" {
		return nil, fmt.Errorf("token endpoint: HTTP %d %s %s", res.StatusCode, tok.Error, clip(tok.Desc, 200))
	}
	parts := strings.Split(tok.IDToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("the ID token isn't a JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("ID token: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, fmt.Errorf("ID token: %w", err)
	}
	if iss, _ := claims["iss"].(string); strings.TrimRight(iss, "/") != cfg.Issuer {
		return nil, fmt.Errorf("ID token is from %q, not %q", iss, cfg.Issuer)
	}
	if !slices.Contains(claimList(claims["aud"]), cfg.ClientID) {
		return nil, errors.New("ID token isn't for this client")
	}
	if exp, _ := claims["exp"].(float64); int64(exp) < time.Now().Unix() {
		return nil, errors.New("ID token has expired")
	}
	if n, _ := claims["nonce"].(string); n != p.nonce {
		return nil, errors.New("ID token is for another sign-in (nonce)")
	}
	return claims, nil
}

// claimList reads a claim that's a string or a list of strings.
func claimList(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// --- settings (admin) --------------------------------------------------------------

func (s *Server) getOIDC(w http.ResponseWriter, r *http.Request) {
	c := s.oidcConfig(r.Context())
	c.Secret = ""
	writeJSON(w, http.StatusOK, map[string]any{"config": c, "redirectUri": origin(r) + "/api/auth/oidc/callback"})
}

func (s *Server) setOIDC(w http.ResponseWriter, r *http.Request) {
	var in oidcConfig
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	in.Issuer = strings.TrimRight(strings.TrimSpace(in.Issuer), "/")
	in.ClientID = strings.TrimSpace(in.ClientID)
	if in.Issuer != "" {
		u, err := url.Parse(in.Issuer)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			writeErr(w, badRequest("the issuer is the provider's address, like https://auth.example.com"))
			return
		}
		if u.Scheme == "http" && !onHomeNetwork(u.Hostname()) {
			writeErr(w, badRequest("the issuer must use https unless it's on your own network: over plain http anyone in between could sign in as anyone"))
			return
		}
		// Check it now, so a typo shows here and not as a broken sign-in page.
		ep, err := s.oidcEndpoints(ctx, in.Issuer)
		if err == nil {
			// The ID token is trusted because it comes from the token endpoint
			// over TLS, so that endpoint must be https too.
			if t, perr := url.Parse(ep.Token); perr != nil || (t.Scheme != "https" && !onHomeNetwork(t.Hostname())) {
				writeErr(w, badRequest("the provider's token endpoint ("+ep.Token+") must use https unless it's on your own network"))
				return
			}
		}
		if err != nil {
			writeErr(w, badRequest("couldn't read the provider's configuration at "+in.Issuer+"/.well-known/openid-configuration: "+err.Error()))
			return
		}
		if in.ClientID == "" {
			writeErr(w, badRequest("the client id the provider gave Couchside is needed"))
			return
		}
	}
	create := "0"
	if in.Create {
		create = "1"
	}
	set := map[string]string{settingOIDCIssuer: in.Issuer, settingOIDCClient: in.ClientID, settingOIDCClaim: strings.TrimSpace(in.Claim),
		settingOIDCCreate: create, settingOIDCAdmins: strings.TrimSpace(in.AdminGroup), settingOIDCLabel: clip(strings.TrimSpace(in.Label), 60)}
	if in.Secret != "" { // left empty, the stored secret stays
		set[settingOIDCSecret] = in.Secret
	}
	if in.Issuer == "" { // turned off: forget the secret too
		set[settingOIDCSecret] = ""
	}
	for k, v := range set {
		if err := s.db.SetSetting(ctx, k, v); err != nil {
			writeErr(w, err)
			return
		}
	}
	slog.Info("single sign-on settings changed", "issuer", in.Issuer, "by", currentUser(ctx).ID)
	w.WriteHeader(http.StatusNoContent)
}

// onHomeNetwork says a host is this machine or on the local network: a
// loopback or private address, localhost, a name without a dot, or a local
// suffix (.local, .lan, .home.arpa, .internal). Plain http is accepted there.
func onHomeNetwork(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" {
		return true
	}
	if _, ok := addrOf(host); ok {
		return !publicAddr(host)
	}
	if !strings.Contains(host, ".") {
		return true
	}
	for _, s := range []string{".local", ".lan", ".home.arpa", ".internal"} {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}
