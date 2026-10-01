package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
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

func TestAccountsOffChangesNothing(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, err := New(d, config.Config{}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	c := newClient(t, ts.URL)
	for _, path := range []string{"/api/home", "/api/libraries", "/api/accounts", "/api/profiles"} {
		if code := c.do("GET", path, nil, nil); code != 200 {
			t.Errorf("%s = %d", path, code)
		}
	}
	var info authInfo
	c.do("GET", "/api/auth", nil, &info)
	if info.Enabled {
		t.Fatal("accounts should be off")
	}
	if code := c.do("POST", "/api/auth/login", map[string]string{"name": "Me", "password": "whatever1"}, nil); code != 400 {
		t.Fatalf("login with accounts off = %d", code)
	}
}
