package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/timothydodd/couchside/internal/auth"
	"github.com/timothydodd/couchside/internal/db"
)

// --- sign-in --------------------------------------------------------------------

// setupNeeded reports whether passwords are required with no admin who has
// one yet, printing the one-time setup code to the log the first time it's
// asked. With passwordless sign-in there's nothing to set up.
func (s *Server) setupNeeded(ctx context.Context) (bool, error) {
	if on, _, err := s.passwordless(ctx); err != nil || on {
		return false, err
	}
	ok, err := s.db.HasAdmin(ctx)
	if err != nil || ok {
		return false, err
	}
	s.auth.mu.Lock()
	defer s.auth.mu.Unlock()
	if s.auth.setupCode == "" {
		s.auth.setupCode = auth.SetupCode()
		slog.Warn("accounts are on but there's no admin yet: open Couchside in a browser and enter this setup code",
			"code", s.auth.setupCode)
	}
	return true, nil
}

type authInfo struct {
	Enabled            bool          `json:"enabled"` // always true (older clients read it)
	Passwordless       bool          `json:"passwordless"`
	PasswordlessLocked bool          `json:"passwordlessLocked"` // COUCHSIDE_AUTH=true requires passwords
	Profiles           []profileStub `json:"profiles"`           // passwordless: every profile to pick from
	HideAdmins         bool          `json:"hideAdmins"`         // admins are left out of profiles; ?admin=1 lists them instead
	SetupRequired      bool          `json:"setupRequired"`
	FirstRun           bool          `json:"firstRun"` // the first-run setup (your name, your media) isn't done
	User               *db.Profile   `json:"user"`
	AccessExpiresAt    int64         `json:"accessExpiresAt,omitempty"`
	ExpiresIn          int64         `json:"expiresIn,omitempty"` // seconds until then, for clients whose clock is off
	SignedIn           []profileStub `json:"signedIn"`            // web: profiles this browser holds a session for
	Insecure           bool          `json:"insecure"`            // this request is plain HTTP from an internet address
	// OIDC is the label of the "sign in with a provider" button, when there is one.
	OIDC string `json:"oidc,omitempty"`
	// RemoteFirstRun: first run, and this visitor isn't on the home network,
	// so claiming the server needs the setup code from the log.
	RemoteFirstRun bool `json:"remoteFirstRun"`
}

type profileStub struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	HasPassword bool   `json:"hasPassword"`
}

// authStatus is open to everyone: it tells an app whether to show sign-in.
func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := authInfo{Enabled: true, SignedIn: []profileStub{}, Profiles: []profileStub{}, Insecure: plainHTTPFromInternet(r)}
	var err error
	if out.Passwordless, out.PasswordlessLocked, err = s.passwordless(ctx); err != nil {
		writeErr(w, err)
		return
	}
	if out.Passwordless {
		if out.Profiles, err = s.pickable(ctx, r.URL.Query().Get("admin") == "1"); err != nil {
			writeErr(w, err)
			return
		}
	}
	if out.HideAdmins, err = s.hideAdmins(ctx); err != nil {
		writeErr(w, err)
		return
	}
	if c := s.oidcConfig(ctx); c.enabled() {
		out.OIDC = c.Label
	}
	if out.SetupRequired, err = s.setupNeeded(ctx); err != nil {
		writeErr(w, err)
		return
	}
	if out.FirstRun, err = s.firstRun(ctx); err != nil {
		writeErr(w, err)
		return
	}
	if out.FirstRun && out.Passwordless && !localClient(r) {
		out.RemoteFirstRun = true
		s.firstRunCode()
	}
	if u, err := s.tokenUser(r); err == nil {
		if p, err := s.db.Profile(ctx, u.ID); err == nil {
			out.User = &p
		}
		if tok, _ := accessToken(r); tok != "" {
			if c, err := s.auth.signer.Verify(tok, time.Now()); err == nil {
				out.AccessExpiresAt = c.Expires
				out.ExpiresIn = max(1, c.Expires-time.Now().Unix())
			}
		}
	}
	now := time.Now().Unix()
	for _, ck := range r.Cookies() {
		if !strings.HasPrefix(ck.Name, refreshPrefix) {
			continue
		}
		if p, err := s.db.PeekRefresh(ctx, auth.HashToken(ck.Value), now); err == nil {
			out.SignedIn = append(out.SignedIn, profileStub{ID: p.ID, Name: p.Name, Color: p.Color, HasPassword: p.HasPassword})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type loginInput struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Client   string `json:"client"` // web (default) | tv
	Device   string `json:"device"` // e.g. "Living room Roku"
	// Code is the second step for a profile that has one: the authenticator
	// app's code, or a recovery code. Left out, the answer says it's needed.
	Code string `json:"code"`
}

// tokens is what a TV app gets from login and refresh; the web gets cookies
// and only user and accessExpiresAt.
type tokens struct {
	User            db.Profile `json:"user"`
	AccessToken     string     `json:"accessToken,omitempty"`
	RefreshToken    string     `json:"refreshToken,omitempty"`
	AccessExpiresAt int64      `json:"accessExpiresAt"`
	// ExpiresIn is seconds until the access token runs out. Clients schedule
	// their renewal from this, not from AccessExpiresAt against their own
	// clock, which may be minutes off.
	ExpiresIn int64  `json:"expiresIn"`
	SessionID string `json:"sessionId"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in loginInput
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
	ip := clientIP(r) // X-Forwarded-For counts only from trusted proxies (realIP)
	nameKey := strings.ToLower(strings.Join(strings.Fields(in.Name), " "))
	if utf8.RuneCountInString(nameKey) > maxProfileName {
		// No account has a name this long. Refuse it before it becomes a
		// limiter key or a log line.
		slog.Warn("sign-in failed", "ip", ip, "name", unknownName, "ua", clip(r.UserAgent(), 200))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong name or password", "code": "bad_credentials"})
		return
	}
	// A sign-in sets cookies, so a page on another site mustn't be able to
	// sign this browser in to a profile of its choosing.
	if in.Client == "web" && !sameOrigin(r) {
		writeErr(w, forbidden("cross-site request refused"))
		return
	}
	done, ok := s.attempt(w, ip, nameKey)
	if !ok {
		return
	}
	// Too long to be anyone's password. Refused here, before the lookup, so a
	// known name and an unknown one answer the same way in the same time
	// (VerifyPassword returns at once for these; DummyVerify doesn't).
	if len(in.Password) > auth.MaxPassword {
		done(true)
		loginFailed(w, r, ip, unknownName)
		return
	}
	p, hash, err := s.db.ProfileForLogin(ctx, in.Name)
	if errors.Is(err, db.ErrNotFound) {
		auth.DummyVerify(in.Password)
	} else if err != nil {
		done(false)
		writeErr(w, err)
		return
	}
	ok = false
	rehash := false
	logName := unknownName // what was typed may be a password in the wrong box
	if err == nil {
		ok, rehash = auth.VerifyPassword(hash, in.Password)
		logName = nameKey
	}
	if ok && p.TwoStep {
		if strings.TrimSpace(in.Code) == "" {
			// The password was right; ask for the code. Not a failed attempt.
			done(false)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "enter the code from your authenticator app", "code": "totp_required"})
			return
		}
		if !s.secondStep(ctx, p, in.Code) {
			done(true)
			slog.Warn("sign-in failed: wrong second-step code", "ip", ip, "name", logName, "ua", clip(r.UserAgent(), 200))
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "that code isn't right", "code": "bad_code"})
			return
		}
	}
	done(!ok)
	if !ok {
		loginFailed(w, r, ip, logName)
		return
	}
	s.auth.byName.Success(nameKey)
	if p.Disabled {
		writeErr(w, forbidden("this account is disabled"))
		return
	}
	if rehash {
		if h, err := auth.HashPassword(in.Password); err == nil {
			_ = s.db.SetPassword(ctx, p.ID, h, p.MustChangePassword)
		}
	}
	s.startSession(w, r, p, in.Client, in.Device)
}

// unknownName stands in the log for a name that isn't a profile's.
const unknownName = "(not an account)"

// attempt claims one password check for this address and account, or answers
// 429 when either must wait. Call done with whether the check failed. The
// claim is made before the check, so parallel requests can't all get in
// ahead of the first failure.
func (s *Server) attempt(w http.ResponseWriter, ip, nameKey string) (done func(failed bool), ok bool) {
	wait := s.auth.byIP.Begin(ip)
	if wait == 0 {
		if wait = s.auth.byName.Begin(nameKey); wait > 0 {
			s.auth.byIP.End(ip, false)
		}
	}
	if wait > 0 {
		retryLater(w, wait)
		return nil, false
	}
	return func(failed bool) {
		s.auth.byIP.End(ip, failed)
		s.auth.byName.End(nameKey, failed)
	}, true
}

func retryLater(w http.ResponseWriter, wait time.Duration) {
	secs := int(wait.Round(time.Second) / time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(max(1, secs)))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error": "too many attempts; try again in " + wait.Round(time.Second).String(), "retryAfter": max(1, secs)})
}

func loginFailed(w http.ResponseWriter, r *http.Request, ip, name string) {
	slog.Warn("sign-in failed", "ip", ip, "name", name, "ua", clip(r.UserAgent(), 200))
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong name or password", "code": "bad_credentials"})
}

// startSession creates a session and answers with its tokens.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, p db.Profile, client, device string) {
	out, err := s.openSession(w, r, p, client, device)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// openSession creates a session for p. For the web it sets the cookies on w;
// a TV app's tokens are in what it returns. Nothing is written to the body,
// so a caller can redirect instead (sign-in through an identity provider).
func (s *Server) openSession(w http.ResponseWriter, r *http.Request, p db.Profile, client, device string) (tokens, error) {
	refresh := auth.NewToken()
	sess := db.Session{ID: auth.NewToken(), ProfileID: p.ID, Client: client, Device: clip(device, 60),
		UserAgent: clip(r.UserAgent(), 300), IP: clientIP(r), ExpiresAt: time.Now().Add(sessionIdle).Unix()}
	if err := s.db.CreateSession(r.Context(), sess, auth.HashToken(refresh)); err != nil {
		return tokens{}, err
	}
	if err := s.db.TrimSessions(r.Context(), p.ID, maxSessions); err != nil {
		slog.Warn("trim sessions", "err", err)
	}
	slog.Info("signed in", "profile", p.Name, "client", client, "ip", sess.IP)
	if plainHTTPFromInternet(r) {
		slog.Warn("sign-in over plain HTTP from the internet: passwords and tokens travel unencrypted; serve Couchside over HTTPS (docs/install.md)",
			"profile", p.Name, "ip", sess.IP)
	}
	return s.grant(w, r, p, sess, refresh), nil
}

// issue sends a fresh access token with the session's (new) refresh token.
func (s *Server) issue(w http.ResponseWriter, r *http.Request, p db.Profile, sess db.Session, refresh string) {
	writeJSON(w, http.StatusOK, s.grant(w, r, p, sess, refresh))
}

// grant makes an access token for the session: cookies for the web, values
// in the result for a TV app.
func (s *Server) grant(w http.ResponseWriter, r *http.Request, p db.Profile, sess db.Session, refresh string) tokens {
	ttl := auth.AccessTTL
	if sess.Client == "tv" {
		ttl = auth.TVAccessTTL
	}
	exp := time.Now().Add(ttl)
	access := s.auth.signer.Sign(auth.Claims{Session: sess.ID, Profile: p.ID, Expires: exp.Unix()})
	out := tokens{User: p, AccessExpiresAt: exp.Unix(), ExpiresIn: int64(ttl / time.Second), SessionID: sess.ID}
	if sess.Client == "tv" {
		out.AccessToken, out.RefreshToken = access, refresh
	} else {
		secure := isHTTPS(r)
		http.SetCookie(w, &http.Cookie{Name: accessCookie, Value: access, Path: "/", MaxAge: int(auth.AccessTTL / time.Second),
			HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
		http.SetCookie(w, &http.Cookie{Name: refreshCookie(p.ID), Value: refresh, Path: refreshPath,
			MaxAge: int(time.Until(time.Unix(sess.ExpiresAt, 0)) / time.Second), HttpOnly: true, Secure: secure,
			SameSite: http.SameSiteStrictMode})
		http.SetCookie(w, &http.Cookie{Name: profileCookie, Value: strconv.FormatInt(p.ID, 10), Path: "/",
			MaxAge: 365 * 24 * 3600, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	}
	return out
}

func refreshCookie(profileID int64) string { return refreshPrefix + strconv.FormatInt(profileID, 10) }

// isHTTPS: X-Forwarded-Proto counts only from a trusted proxy (see realIP).
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || viaHTTPS(r)
}

// clip cuts s to at most n bytes, at a character boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

type refreshInput struct {
	RefreshToken string `json:"refreshToken"` // TV apps
	ProfileID    int64  `json:"profileId"`    // web: switch to this signed-in profile
}

// refresh trades a refresh token for new tokens. The web sends its cookie for
// the active profile, or for profileId to switch profiles.
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var in refreshInput
	if r.ContentLength != 0 {
		if err := decode(r, &in); err != nil {
			writeErr(w, err)
			return
		}
	}
	web := in.RefreshToken == ""
	cookieName := ""
	tok := in.RefreshToken
	if web {
		pid := in.ProfileID
		if pid == 0 {
			if c, err := r.Cookie(profileCookie); err == nil {
				pid, _ = strconv.ParseInt(c.Value, 10, 64)
			}
		}
		cookieName = refreshCookie(pid)
		if c, err := r.Cookie(cookieName); err == nil {
			tok = c.Value
		}
		// Refresh cookies are SameSite=Strict, but check anyway: this sets cookies.
		if !sameOrigin(r) {
			writeErr(w, forbidden("cross-site request refused"))
			return
		}
	}
	if tok == "" {
		unauthorized(w, "sign in to continue")
		return
	}
	ctx := r.Context()
	next := auth.NewToken()
	idle := func(string) int64 { return int64(sessionIdle / time.Second) }
	sess, res, err := s.db.RotateRefresh(ctx, auth.HashToken(tok), auth.HashToken(next), time.Now().Unix(), idle, clientIP(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	switch res {
	case db.RefreshStale:
		// Another tab refreshed with this cookie a moment ago; the new one is
		// already in the browser, so the caller just retries.
		writeJSON(w, http.StatusConflict, map[string]string{"error": "refreshed elsewhere; try again", "code": "refresh_stale"})
		return
	case db.RefreshReused:
		slog.Warn("a used refresh token was presented again: the session was ended in case it was stolen",
			"profile", sess.ProfileID, "session_ip", sess.IP, "ip", clientIP(r))
		s.auth.sessions.forget(sess.ID)
		fallthrough
	case db.RefreshUnknown:
		if web {
			expire(w, r, cookieName, refreshPath)
		}
		unauthorized(w, "sign in again")
		return
	}
	p, err := s.db.Profile(ctx, sess.ProfileID)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.issue(w, r, p, sess, next)
}

func expire(w http.ResponseWriter, r *http.Request, name, path string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: path, MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r)})
}

type setupInput struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// setup creates the first admin with the code from the log, then signs them in.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	need, err := s.setupNeeded(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !need {
		writeErr(w, badRequest("this server is already set up"))
		return
	}
	var in setupInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if !sameOrigin(r) {
		writeErr(w, forbidden("cross-site request refused"))
		return
	}
	ip := clientIP(r)
	done, ok := s.attempt(w, ip, "setup")
	if !ok {
		return
	}
	s.auth.mu.Lock()
	code := s.auth.setupCode
	s.auth.mu.Unlock()
	wrong := !auth.SameCode(in.Code, code)
	done(wrong)
	if wrong {
		slog.Warn("wrong setup code", "ip", ip)
		writeErr(w, httpError{http.StatusUnauthorized, "that setup code isn't right; it's in the server log"})
		return
	}
	pi := profileInput{Name: in.Name}
	if err := pi.validate(); err != nil {
		writeErr(w, err)
		return
	}
	if err := auth.CheckPassword(in.Password); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.db.SetupAdmin(ctx, pi.Name, hash)
	if err != nil {
		writeErr(w, profileErr(err))
		return
	}
	s.auth.mu.Lock()
	s.auth.setupCode = ""
	s.auth.mu.Unlock()
	slog.Info("first admin set up", "profile", p.Name, "ip", ip)
	s.startSession(w, r, p, "web", "")
}

type logoutInput struct {
	Everywhere bool `json:"everywhere"`
}

// logout ends this session (or all of this profile's sessions).
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	var in logoutInput
	if r.ContentLength != 0 {
		if err := decode(r, &in); err != nil {
			writeErr(w, err)
			return
		}
	}
	var err error
	if in.Everywhere {
		err = s.db.DeleteSessions(r.Context(), u.ID, "")
		s.auth.sessions.forgetAll()
	} else {
		err = s.db.DeleteSession(r.Context(), u.Session)
		s.auth.sessions.forget(u.Session)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if u.Cookie {
		expire(w, r, accessCookie, "/")
		expire(w, r, refreshCookie(u.ID), refreshPath)
		expire(w, r, profileCookie, "/")
	}
	w.WriteHeader(http.StatusNoContent)
}
