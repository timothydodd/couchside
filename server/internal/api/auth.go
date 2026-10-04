package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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
