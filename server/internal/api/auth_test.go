package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// authServer starts an API server with accounts on.
func authServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, config.Config{Auth: true, DataDir: dir}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

type testClient struct {
	t      *testing.T
	base   string
	http   *http.Client
	bearer string
	origin string
	header map[string]string // extra request headers
}

func newClient(t *testing.T, base string) *testClient {
	jar, _ := cookiejar.New(nil)
	return &testClient{t: t, base: base, http: &http.Client{Jar: jar}}
}

// do sends a request and decodes a JSON reply into out (when given).
func (c *testClient) do(method, path string, body any, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	for k, v := range c.header {
		req.Header.Set(k, v)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func TestAccountsFlow(t *testing.T) {
	s, ts := authServer(t)
	web := newClient(t, ts.URL)

	// Nothing works before signing in, but the auth status does.
	if code := web.do("GET", "/api/home", nil, nil); code != 401 {
		t.Fatalf("home before setup = %d", code)
	}
	if code := web.do("GET", "/api/files/1/stream", nil, nil); code != 401 {
		t.Fatalf("stream before setup = %d", code)
	}
	// Artwork is open, so TV image nodes needn't send a token (404: no such item).
	if code := web.do("GET", "/api/artwork/items/1/poster", nil, nil); code == 401 {
		t.Fatal("artwork should not need a token")
	}
	var info authInfo
	web.do("GET", "/api/auth", nil, &info)
	if !info.Enabled || !info.SetupRequired || info.User != nil {
		t.Fatalf("auth info = %+v", info)
	}
	if code := web.do("POST", "/api/auth/setup", map[string]string{"code": "WRONG", "name": "Me", "password": "long enough"}, nil); code != 401 {
		t.Fatalf("setup with wrong code = %d", code)
	}
	s.auth.mu.Lock()
	code := s.auth.setupCode
	s.auth.mu.Unlock()
	var tk tokens
	if c := web.do("POST", "/api/auth/setup", map[string]string{"code": code, "name": "Me", "password": "long enough"}, &tk); c != 200 {
		t.Fatalf("setup = %d", c)
	}
	if tk.User.Role != "admin" || tk.User.ID != 1 || tk.AccessToken != "" {
		t.Fatalf("setup tokens = %+v (web gets cookies, not tokens)", tk)
	}
	if c := web.do("GET", "/api/home", nil, nil); c != 200 {
		t.Fatalf("home after setup = %d", c)
	}
	if c := web.do("POST", "/api/auth/setup", map[string]string{"code": code, "name": "X", "password": "long enough"}, nil); c != 400 {
		t.Fatalf("second setup = %d", c)
	}

	// The admin adds a user, who may not record.
	var kid db.Profile
	if c := web.do("POST", "/api/accounts", map[string]any{"name": "Kid", "password": "temporary pw", "role": "user"}, &kid); c != 201 {
		t.Fatalf("create account = %d", c)
	}

	// The kid signs in on a TV with the temporary password and must change it first.
	tv := newClient(t, ts.URL)
	if c := tv.do("POST", "/api/auth/login", map[string]string{"name": "kid", "password": "temporary pw", "client": "tv"}, &tk); c != 200 {
		t.Fatalf("tv login = %d", c)
	}
	if tk.AccessToken == "" || tk.RefreshToken == "" || !tk.User.MustChangePassword {
		t.Fatalf("tv tokens = %+v", tk)
	}
	tv.bearer = tk.AccessToken
	if c := tv.do("GET", "/api/home", nil, nil); c != 403 {
		t.Fatalf("home before changing a temporary password = %d", c)
	}
	if c := tv.do("POST", "/api/auth/password", map[string]string{"current": "temporary pw", "password": "kid's own pw"}, nil); c != 204 {
		t.Fatalf("change password = %d", c)
	}
	if c := tv.do("GET", "/api/home", nil, nil); c != 200 {
		t.Fatalf("home after changing password = %d", c)
	}

	// Users can't reach admin routes or record.
	if c := tv.do("GET", "/api/libraries", nil, nil); c != 403 {
		t.Fatalf("user libraries = %d", c)
	}
	if c := tv.do("GET", "/api/accounts", nil, nil); c != 403 {
		t.Fatalf("user accounts = %d", c)
	}
	if c := tv.do("POST", "/api/dvr/recordings", map[string]int{"programId": 1}, nil); c != 403 {
		t.Fatalf("user record = %d", c)
	}
	var profs struct{ Profiles []db.Profile }
	tv.do("GET", "/api/profiles", nil, &profs)
	if len(profs.Profiles) != 1 || profs.Profiles[0].Name != "Kid" {
		t.Fatalf("a user should only see themselves: %+v", profs)
	}
	if c := tv.do("PATCH", "/api/profiles/1/prefs", map[string]string{"theme": "dark"}, nil); c != 404 {
		t.Fatalf("editing someone else's prefs = %d", c)
	}

	// Refresh tokens rotate, and the old one stops working.
	old := tk.RefreshToken
	var tk2 tokens
	tv.bearer = ""
	if c := tv.do("POST", "/api/auth/refresh", map[string]string{"refreshToken": old}, &tk2); c != 200 || tk2.RefreshToken == old {
		t.Fatalf("refresh = %d %+v", c, tk2)
	}
	if c := tv.do("POST", "/api/auth/refresh", map[string]string{"refreshToken": old}, nil); c != 409 {
		t.Fatalf("immediate replay = %d, want 409 (stale race)", c)
	}
	// (A replay after the grace minute ends the session: see db.TestRefreshRotation.)
	tv.bearer = tk2.AccessToken
	if c := tv.do("GET", "/api/home", nil, nil); c != 200 {
		t.Fatalf("home with the refreshed token = %d", c)
	}

	// Cross-site writes with the cookie are refused.
	web.origin = "http://evil.example"
	if c := web.do("POST", "/api/items/1/watched", map[string]bool{"watched": true}, nil); c != 403 {
		t.Fatalf("cross-site POST = %d", c)
	}
	web.origin = ""

	// Signing a device out in Settings stops it at once.
	tv2 := newClient(t, ts.URL)
	if c := tv2.do("POST", "/api/auth/login", map[string]string{"name": "Kid", "password": "kid's own pw", "client": "tv"}, &tk); c != 200 {
		t.Fatalf("second tv login = %d", c)
	}
	tv2.bearer = tk.AccessToken
	if c := tv2.do("GET", "/api/home", nil, nil); c != 200 {
		t.Fatalf("second tv = %d", c)
	}
	if c := web.do("DELETE", "/api/auth/sessions/"+tk.SessionID, nil, nil); c != 204 {
		t.Fatalf("admin ends session = %d", c)
	}
	if c := tv2.do("GET", "/api/home", nil, nil); c != 401 {
		t.Fatalf("ended session still works: %d", c)
	}

	// The web refreshes with its cookie, and logout ends it.
	if c := web.do("POST", "/api/auth/refresh", nil, nil); c != 200 {
		t.Fatalf("web refresh = %d", c)
	}
	web.do("GET", "/api/auth", nil, &info)
	if info.User == nil || len(info.SignedIn) != 1 || info.SignedIn[0].Name != "Me" {
		t.Fatalf("auth info after sign-in = %+v", info)
	}
	if c := web.do("POST", "/api/auth/logout", nil, nil); c != 204 {
		t.Fatalf("logout = %d", c)
	}
	if c := web.do("GET", "/api/home", nil, nil); c != 401 {
		t.Fatalf("home after logout = %d", c)
	}
	if c := web.do("POST", "/api/auth/refresh", map[string]int64{"profileId": 1}, nil); c != 401 {
		t.Fatalf("refresh after logout = %d", c)
	}

	// Wrong passwords get throttled.
	attacker := newClient(t, ts.URL)
	last := 0
	for i := 0; i < 7; i++ {
		last = attacker.do("POST", "/api/auth/login", map[string]string{"name": "Me", "password": "guess guess"}, nil)
	}
	if last != 429 {
		t.Fatalf("after 7 wrong passwords: %d, want 429", last)
	}
}

func TestPasswordless(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, err := New(d, config.Config{DataDir: dir}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	c := newClient(t, ts.URL)

	// Accounts are always on: nothing answers without a session.
	if code := c.do("GET", "/api/home", nil, nil); code != 401 {
		t.Fatalf("home without a session = %d", code)
	}
	// A server where nobody has a password is passwordless, and lists its profiles.
	var info authInfo
	c.do("GET", "/api/auth", nil, &info)
	if !info.Passwordless || info.SetupRequired || len(info.Profiles) != 1 || info.Profiles[0].Name != "Me" {
		t.Fatalf("auth info = %+v", info)
	}
	// Picking a profile signs in with real tokens; the oldest profile is the admin.
	var tk tokens
	if code := c.do("POST", "/api/auth/pick", map[string]any{"profileId": info.Profiles[0].ID}, &tk); code != 200 {
		t.Fatalf("pick = %d", code)
	}
	if tk.User.Role != "admin" || !tk.User.CanRecord {
		t.Fatalf("picked user = %+v", tk.User)
	}
	if code := c.do("GET", "/api/libraries", nil, nil); code != 200 {
		t.Fatalf("admin route after picking = %d", code)
	}
	// Accounts can be made without a password, and picked.
	var kid db.Profile
	if code := c.do("POST", "/api/accounts", map[string]any{"name": "Kid"}, &kid); code != 201 || kid.HasPassword || kid.MustChangePassword {
		t.Fatalf("passwordless account = %d %+v", code, kid)
	}
	// An admin who sets a password is asked for it, even in passwordless mode.
	if code := c.do("POST", "/api/auth/password", map[string]string{"current": "", "password": "admin password"}, nil); code != 204 {
		t.Fatalf("first password = %d", code)
	}
	tv := newClient(t, ts.URL)
	var res map[string]string
	if code := tv.do("POST", "/api/auth/pick", map[string]any{"profileId": 1, "client": "tv"}, &res); code != 401 || res["code"] != "password_required" {
		t.Fatalf("picking a profile with a password = %d %v", code, res)
	}
	if code := tv.do("POST", "/api/auth/pick", map[string]any{"profileId": kid.ID, "client": "tv"}, &tk); code != 200 || tk.AccessToken == "" {
		t.Fatalf("tv pick = %d %+v", code, tk)
	}
	// Turning passwordless off: picking stops working, passwords don't.
	if code := c.do("PUT", "/api/settings/passwordless", map[string]bool{"enabled": false}, nil); code != 204 {
		t.Fatalf("passwordless off = %d", code)
	}
	if code := tv.do("POST", "/api/auth/pick", map[string]any{"profileId": kid.ID, "client": "tv"}, nil); code != 403 {
		t.Fatalf("pick with passwordless off = %d", code)
	}
	c.do("GET", "/api/auth", nil, &info)
	if info.Passwordless || len(info.Profiles) != 0 {
		t.Fatalf("auth info with passwordless off = %+v (no profile list for strangers)", info)
	}
	if code := newClient(t, ts.URL).do("POST", "/api/auth/login", map[string]string{"name": "Me", "password": "admin password"}, nil); code != 200 {
		t.Fatalf("password login = %d", code)
	}
}

func TestPasswordLocked(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, err := New(d, config.Config{DataDir: dir}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	if code := admin.do("POST", "/api/auth/pick", map[string]any{"profileId": 1}, nil); code != 200 {
		t.Fatalf("admin pick = %d", code)
	}

	// A shared Guest profile that can't set its own password.
	var guest db.Profile
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Guest", "passwordLocked": true}, &guest); code != 201 || !guest.PasswordLocked {
		t.Fatalf("locked account = %d %+v", code, guest)
	}
	var tk tokens
	g := newClient(t, ts.URL)
	if code := g.do("POST", "/api/auth/pick", map[string]any{"profileId": guest.ID, "client": "tv"}, &tk); code != 200 {
		t.Fatalf("guest pick = %d", code)
	}
	g.bearer = tk.AccessToken
	if code := g.do("POST", "/api/auth/password", map[string]string{"current": "", "password": "mine now, ha"}, nil); code != 403 {
		t.Fatalf("locked account set its own password: %d", code)
	}

	// An admin's password for a locked account isn't temporary: it can't be changed by the account.
	if code := admin.do("POST", fmt.Sprintf("/api/accounts/%d/password", guest.ID), map[string]string{"password": "guest password"}, nil); code != 204 {
		t.Fatalf("admin reset = %d", code)
	}
	if p, _ := d.Profile(context.Background(), guest.ID); p.MustChangePassword || !p.HasPassword {
		t.Fatalf("locked account after reset = %+v", p)
	}

	// Unlocked, it can.
	update := map[string]any{"name": "Guest", "role": "user", "passwordLocked": false}
	if code := admin.do("PUT", fmt.Sprintf("/api/accounts/%d", guest.ID), update, nil); code != 200 {
		t.Fatalf("unlock = %d", code)
	}
	g2 := newClient(t, ts.URL)
	if code := g2.do("POST", "/api/auth/login", map[string]any{"name": "Guest", "password": "guest password", "client": "tv"}, &tk); code != 200 {
		t.Fatalf("guest login = %d", code)
	}
	g2.bearer = tk.AccessToken
	if code := g2.do("POST", "/api/auth/password", map[string]string{"current": "guest password", "password": "a better one"}, nil); code != 204 {
		t.Fatalf("unlocked account changing its password = %d", code)
	}

	// Admins can't be locked out of their own password.
	var me db.Profile
	admin.do("PUT", "/api/accounts/1", map[string]any{"name": "Me", "role": "admin", "passwordLocked": true}, &me)
	if me.PasswordLocked {
		t.Fatal("an admin was locked")
	}
}

// A forged X-Forwarded-For from a peer that isn't a trusted proxy doesn't
// change the address the sign-in throttle counts.
func TestForgedForwardedForDoesNotDodgeThrottle(t *testing.T) {
	_, ts := authServer(t)
	c := newClient(t, ts.URL)
	code := 0
	for i := 0; i < 8; i++ {
		c.header = map[string]string{"X-Forwarded-For": "198.51.100." + strconv.Itoa(i), "X-Real-IP": "198.51.100." + strconv.Itoa(i)}
		// A different name each time, so only the per-address limit can apply.
		code = c.do("POST", "/api/auth/login", map[string]string{"name": "nobody" + strconv.Itoa(i), "password": "wrong"}, nil)
	}
	if code != http.StatusTooManyRequests {
		t.Fatalf("8th failed sign-in with a new forged address each time = %d, want 429", code)
	}
}

// Behind a reverse proxy that rewrites Host (nginx's default proxy_pass), the
// browser's Origin never matches Host. Refreshing and other cookie writes
// must still work, going by Sec-Fetch-Site, or the web app signs out every
// time its access token runs out.
func TestCookieWritesBehindHostRewritingProxy(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, err := New(d, config.Config{DataDir: dir}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	c := newClient(t, ts.URL)
	c.origin = "https://demo.example" // what the browser shows; Host is the upstream address
	c.header = map[string]string{"Sec-Fetch-Site": "same-origin"}

	var info authInfo
	c.do("GET", "/api/auth", nil, &info)
	if code := c.do("POST", "/api/auth/pick", map[string]any{"profileId": info.Profiles[0].ID, "client": "web"}, nil); code != 200 {
		t.Fatalf("pick = %d", code)
	}
	for i := 0; i < 2; i++ {
		if code := c.do("POST", "/api/auth/refresh", map[string]any{}, nil); code != 200 {
			t.Fatalf("refresh %d through the proxy = %d, want 200", i, code)
		}
	}
	if code := c.do("PATCH", "/api/profiles/1/prefs", map[string]any{"autoplayNext": false}, nil); code != 200 && code != 204 {
		t.Fatalf("cookie write through the proxy = %d", code)
	}

	// A browser saying the request is cross-site is still refused, whatever Origin says.
	c.origin = ts.URL
	c.header = map[string]string{"Sec-Fetch-Site": "cross-site"}
	if code := c.do("POST", "/api/auth/refresh", map[string]any{}, nil); code != 403 {
		t.Fatalf("cross-site refresh = %d, want 403", code)
	}
	// Without Sec-Fetch-Site, Origin must match Host.
	c.origin = "https://evil.example"
	c.header = nil
	if code := c.do("POST", "/api/auth/refresh", map[string]any{}, nil); code != 403 {
		t.Fatalf("mismatched Origin without Sec-Fetch-Site = %d, want 403", code)
	}
}
