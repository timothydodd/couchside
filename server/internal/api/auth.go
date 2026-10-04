package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/auth"
	"github.com/timothydodd/couchside/internal/db"
)

// Accounts. Every /api request carries a short-lived
// signed access token: a cookie for the web (<video>, <img> and hls.js can't
// send headers) or "Authorization: Bearer" for TV apps. Long-lived refresh
// tokens are random, stored hashed, and replaced on every use; a replayed one
// ends its session. The web keeps one refresh cookie per signed-in profile
// (couchside_refresh_<id>, sent only to /api/auth) so a shared browser can
// switch between them, and couchside_profile names the active one.
const (
	accessCookie  = "couchside_access"
	refreshPrefix = "couchside_refresh_"
	refreshPath   = "/api/auth"

	// A session (its refresh token) ends after this long unused. Every refresh
	// pushes it out again, so a device that's used now and then stays signed in.
	// A year: Couchside is a home server, and nobody should be retyping a
	// password on a TV remote. Signing out, a password change or an admin ends
	// sessions sooner.
	sessionIdle = 365 * 24 * time.Hour

	maxSessions    = 50               // per profile; signing in again drops the least recently used
	sessionRecheck = 30 * time.Second // how stale the cached session/role check may be
)

// authState is the server's accounts machinery.
type authState struct {
	signer   *auth.Signer
	byIP     *auth.Limiter
	byName   *auth.Limiter
	picks    *auth.Limiter // passwordless picks per address
	sessions sessionCache

	mu        sync.Mutex
	setupCode string
}

func newAuthState(key []byte) *authState {
	return &authState{
		signer: auth.NewSigner(key),
		// Five free tries per address, then 2s doubling to 15 minutes.
		byIP: auth.NewLimiter(5, 2*time.Second, 15*time.Minute, 15*time.Minute),
		// Per account, wherever the guesses come from: 30s doubling to 15 minutes.
		byName: auth.NewLimiter(5, 30*time.Second, 15*time.Minute, 15*time.Minute),
		// Each pick makes a year-long session: 20 per address per 10 minutes, then waits.
		picks:    auth.NewLimiter(20, 5*time.Second, 5*time.Minute, 10*time.Minute),
		sessions: sessionCache{m: map[string]cachedSession{}},
	}
}

// sessionCache saves a database lookup on every request (an HLS player makes
// several a second). Changes made here forget entries at once; changes made
// elsewhere (the reset-password command) are seen within sessionRecheck.
type sessionCache struct {
	mu sync.Mutex
	m  map[string]cachedSession
}

type cachedSession struct {
	su      db.SessionUser
	fetched time.Time
}

func (c *sessionCache) get(ctx context.Context, d *db.DB, sid string, now time.Time) (db.SessionUser, error) {
	c.mu.Lock()
	e, ok := c.m[sid]
	c.mu.Unlock()
	if ok && now.Sub(e.fetched) < sessionRecheck && e.su.ExpiresAt > now.Unix() {
		return e.su, nil
	}
	su, err := d.SessionUser(ctx, sid, now.Unix())
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		delete(c.m, sid)
		return su, err
	}
	if len(c.m) > 5000 {
		clear(c.m)
	}
	c.m[sid] = cachedSession{su, now}
	return su, nil
}

func (c *sessionCache) forget(sid string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, sid)
}

// forgetAll is used after a profile's role, password or status changes.
func (c *sessionCache) forgetAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.m)
}

// --- the signed-in user ---------------------------------------------------------

// user is who a request acts as: the signed-in profile and what it may do.
type user struct {
	ID         int64
	Admin      bool
	CanRecord  bool
	MustChange bool
	Session    string
	Cookie     bool // authenticated by cookie (the web) rather than a bearer token
}

type userKey struct{}

func currentUser(ctx context.Context) user {
	u, _ := ctx.Value(userKey{}).(user)
	return u
}

func forbidden(msg string) error { return httpError{http.StatusForbidden, msg} }

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="couchside"`)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg, "code": "unauthorized"})
}

// authenticate puts the request's user (and profile) in its context, or
// refuses it with 401 when accounts are on and it has no valid access token.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		u, err := s.tokenUser(r)
		if err != nil {
			unauthorized(w, "sign in to continue")
			return
		}
		// Cookies ride along on cross-site requests, so changes must come from our own pages.
		if u.Cookie && !safeMethod(r.Method) && !sameOrigin(r) {
			writeErr(w, forbidden("cross-site request refused"))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(db.WithProfile(ctx, u.ID), userKey{}, u)))
	})
}

// tokenUser checks the request's access token and its session.
func (s *Server) tokenUser(r *http.Request) (user, error) {
	tok, cookie := accessToken(r)
	if tok == "" {
		return user{}, auth.ErrBadToken
	}
	now := time.Now()
	c, err := s.auth.signer.Verify(tok, now)
	if err != nil {
		return user{}, err
	}
	su, err := s.auth.sessions.get(r.Context(), s.db, c.Session, now)
	if err != nil || su.Profile.ID != c.Profile {
		return user{}, auth.ErrBadToken
	}
	p := su.Profile
	return user{ID: p.ID, Admin: p.Role == "admin", CanRecord: p.Role == "admin" || p.CanRecord,
		MustChange: p.MustChangePassword, Session: c.Session, Cookie: cookie}, nil
}

func accessToken(r *http.Request) (string, bool) {
	if h := r.Header.Get("Authorization"); h != "" {
		if t, ok := strings.CutPrefix(h, "Bearer "); ok {
			return strings.TrimSpace(t), false
		}
		return "", false
	}
	if c, err := r.Cookie(accessCookie); err == nil {
		return c.Value, true
	}
	return "", false
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// sameOrigin reports whether a request comes from Couchside's own pages.
// Browsers send Sec-Fetch-Site on every request and pages can't set it, so
// it decides when present. That also holds behind a reverse proxy that
// rewrites Host (nginx's default proxy_pass does), where Origin and Host
// never match and every refresh was refused, signing the web app out once
// its access token ran out. Origin against Host is the fallback for clients
// that don't send it.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
	default: // same-site, cross-site
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
	return true
}

// passwordCurrent holds back everything but the account routes until a
// temporary password has been replaced.
func (s *Server) passwordCurrent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r.Context()).MustChange {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "choose a new password first", "code": "password_change_required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// adminOnly guards settings, libraries, management and accounts.
func adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r.Context()).Admin {
			writeErr(w, forbidden("only an admin can do that"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// recorders guards scheduling recordings and series rules.
func recorders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r.Context()).CanRecord {
			writeErr(w, forbidden("recording isn't allowed for this profile; an admin can turn it on"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// mayManage says whether u may change something owned by owner (0 = admins only).
func (u user) mayManage(owner int64) bool { return u.Admin || (owner != 0 && owner == u.ID) }

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
	SetupRequired      bool          `json:"setupRequired"`
	User               *db.Profile   `json:"user"`
	AccessExpiresAt    int64         `json:"accessExpiresAt,omitempty"`
	ExpiresIn          int64         `json:"expiresIn,omitempty"` // seconds until then, for clients whose clock is off
	SignedIn           []profileStub `json:"signedIn"`            // web: profiles this browser holds a session for
	Insecure           bool          `json:"insecure"`            // this request is plain HTTP from an internet address
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
		if out.Profiles, err = s.pickable(ctx); err != nil {
			writeErr(w, err)
			return
		}
	}
	if out.SetupRequired, err = s.setupNeeded(ctx); err != nil {
		writeErr(w, err)
		return
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

// startSession creates a session and hands out its tokens.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, p db.Profile, client, device string) {
	refresh := auth.NewToken()
	sess := db.Session{ID: auth.NewToken(), ProfileID: p.ID, Client: client, Device: clip(device, 60),
		UserAgent: clip(r.UserAgent(), 300), IP: clientIP(r), ExpiresAt: time.Now().Add(sessionIdle).Unix()}
	if err := s.db.CreateSession(r.Context(), sess, auth.HashToken(refresh)); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.TrimSessions(r.Context(), p.ID, maxSessions); err != nil {
		slog.Warn("trim sessions", "err", err)
	}
	slog.Info("signed in", "profile", p.Name, "client", client, "ip", sess.IP)
	if plainHTTPFromInternet(r) {
		slog.Warn("sign-in over plain HTTP from the internet: passwords and tokens travel unencrypted; serve Couchside over HTTPS (docs/install.md)",
			"profile", p.Name, "ip", sess.IP)
	}
	s.issue(w, r, p, sess, refresh)
}

// issue sends a fresh access token with the session's (new) refresh token.
func (s *Server) issue(w http.ResponseWriter, r *http.Request, p db.Profile, sess db.Session, refresh string) {
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
	writeJSON(w, http.StatusOK, out)
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

type passwordInput struct {
	Current  string `json:"current"`
	Password string `json:"password"`
}

// changePassword sets the user's own password and signs out their other devices.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := currentUser(ctx)
	var in passwordInput
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.db.Profile(ctx, u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if p.PasswordLocked && p.Role != "admin" {
		writeErr(w, httpError{http.StatusForbidden, "only an admin can change this account's password"})
		return
	}
	ip := clientIP(r)
	nameKey := strings.ToLower(p.Name)
	done, ok := s.attempt(w, ip, nameKey)
	if !ok {
		return
	}
	hash, err := s.db.PasswordHash(ctx, u.ID)
	if err != nil {
		done(false)
		writeErr(w, err)
		return
	}
	// A profile without a password (passwordless sign-in) sets its first one.
	ok, _ = auth.VerifyPassword(hash, in.Current)
	wrong := hash != "" && !ok
	done(wrong)
	if wrong {
		slog.Warn("password change: wrong current password", "ip", ip, "name", nameKey)
		writeErr(w, badRequest("your current password isn't right"))
		return
	}
	if err := auth.CheckPassword(in.Password); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	if hash != "" && in.Password == in.Current {
		writeErr(w, badRequest("choose a different password"))
		return
	}
	newHash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.SetPassword(ctx, u.ID, newHash, false); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.db.DeleteSessions(ctx, u.ID, u.Session); err != nil {
		writeErr(w, err)
		return
	}
	s.auth.sessions.forgetAll()
	slog.Info("password changed", "profile", p.Name, "ip", ip)
	w.WriteHeader(http.StatusNoContent)
}

type sessionView struct {
	db.Session
	Current bool `json:"current"`
}

// mySessions lists the user's signed-in devices.
func (s *Server) mySessions(w http.ResponseWriter, r *http.Request) {
	s.listSessions(w, r, currentUser(r.Context()).ID)
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request, profileID int64) {
	ss, err := s.db.Sessions(r.Context(), profileID, time.Now().Unix())
	if err != nil {
		writeErr(w, err)
		return
	}
	cur := currentUser(r.Context()).Session
	out := make([]sessionView, 0, len(ss))
	for _, x := range ss {
		out = append(out, sessionView{x, x.ID == cur})
	}
	writeJSON(w, http.StatusOK, out)
}

// endSession signs one device out: your own, or anyone's for an admin.
func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := currentUser(ctx)
	sess, err := s.db.Session(ctx, chi.URLParam(r, "sid"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if !u.mayManage(sess.ProfileID) {
		writeErr(w, db.ErrNotFound)
		return
	}
	if err := s.db.DeleteSession(ctx, sess.ID); err != nil {
		writeErr(w, err)
		return
	}
	s.auth.sessions.forget(sess.ID)
	w.WriteHeader(http.StatusNoContent)
}

// endOtherSessions signs the user out everywhere but here.
func (s *Server) endOtherSessions(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if err := s.db.DeleteSessions(r.Context(), u.ID, u.Session); err != nil {
		writeErr(w, err)
		return
	}
	s.auth.sessions.forgetAll()
	w.WriteHeader(http.StatusNoContent)
}

// pruneSessions drops expired sessions now and then.
func (s *Server) pruneSessions(ctx context.Context) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		if err := s.db.PruneSessions(ctx, time.Now().Unix()); err != nil && ctx.Err() == nil {
			slog.Warn("prune sessions", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
