package api

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// A new passwordless server: the browser names the admin, maybe gives it a
// password, is signed in, and finishing marks setup done for good.
func TestFirstRun(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, config.Config{DataDir: dir}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	web := newClient(t, ts.URL)

	var info authInfo
	web.do("GET", "/api/auth", nil, &info)
	if !info.FirstRun || !info.Passwordless || info.SetupRequired {
		t.Fatalf("new server = %+v", info)
	}
	if c := web.do("POST", "/api/auth/welcome", map[string]string{"name": "", "password": ""}, nil); c != 400 {
		t.Fatalf("no name = %d", c)
	}
	if c := web.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim", "password": "short"}, nil); c != 400 {
		t.Fatalf("weak password = %d", c)
	}
	var tk tokens
	if c := web.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim", "password": "long enough"}, &tk); c != 200 {
		t.Fatalf("welcome = %d", c)
	}
	if tk.User.Name != "Tim" || tk.User.Role != "admin" || !tk.User.HasPassword {
		t.Fatalf("welcomed = %+v", tk.User)
	}
	// Someone else can't take it over now it has a password.
	if c := newClient(t, ts.URL).do("POST", "/api/auth/welcome", map[string]string{"name": "Mallory"}, nil); c != 403 {
		t.Fatalf("second welcome = %d", c)
	}
	if c := web.do("POST", "/api/setup/complete", nil, nil); c != 204 {
		t.Fatalf("finish = %d", c)
	}
	web.do("GET", "/api/auth", nil, &info)
	if info.FirstRun {
		t.Fatal("still first run after finishing")
	}
	if c := newClient(t, ts.URL).do("POST", "/api/auth/welcome", map[string]string{"name": "Mallory"}, nil); c != 403 {
		t.Fatalf("welcome after setup = %d", c)
	}
}

// Until the first run is done, an unclaimed passwordless server can only be
// claimed from the home network, or with the setup code from the log (and
// then a password is required).
func TestFirstRunRefusedFromInternet(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, config.Config{DataDir: dir, TrustedProxies: []string{"127.0.0.1"}}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	from := func(ip string) *testClient {
		c := newClient(t, ts.URL)
		c.header = map[string]string{"X-Forwarded-For": ip}
		return c
	}
	outsider := from("203.0.113.9")

	var info authInfo
	outsider.do("GET", "/api/auth", nil, &info)
	if !info.FirstRun || !info.RemoteFirstRun {
		t.Fatalf("outsider sees %+v, want remoteFirstRun", info)
	}
	var e struct{ Error, Code string }
	if c := outsider.do("POST", "/api/auth/welcome", map[string]string{"name": "Mallory"}, &e); c != 403 || e.Code != "private_only" {
		t.Fatalf("outsider welcome = %d %+v, want 403 private_only", c, e)
	}
	if c := outsider.do("POST", "/api/auth/pick", map[string]any{"profileId": 1}, &e); c != 403 || e.Code != "private_only" {
		t.Fatalf("outsider pick = %d %+v, want 403 private_only", c, e)
	}
	if c := outsider.do("POST", "/api/auth/welcome", map[string]string{"name": "Mallory", "code": "WRONG-CODE-0000"}, nil); c != 401 {
		t.Fatalf("wrong code = %d, want 401", c)
	}
	s.auth.mu.Lock()
	code := s.auth.setupCode
	s.auth.mu.Unlock()
	if code == "" {
		t.Fatal("no setup code was made for the first run")
	}
	if c := outsider.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim", "code": code}, nil); c != 400 {
		t.Fatalf("remote claim without a password = %d, want 400", c)
	}
	var tk tokens
	if c := outsider.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim", "password": "long enough", "code": code}, &tk); c != 200 {
		t.Fatalf("remote claim with the code = %d", c)
	}
	if tk.User.Name != "Tim" || !tk.User.HasPassword {
		t.Fatalf("claimed = %+v", tk.User)
	}
	s.auth.mu.Lock()
	spent := s.auth.setupCode == ""
	s.auth.mu.Unlock()
	if !spent {
		t.Fatal("the setup code still works after claiming")
	}
}

// From the home network the first run needs no code.
func TestFirstRunFromHomeNetwork(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, config.Config{DataDir: dir, TrustedProxies: []string{"127.0.0.1"}}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	home := newClient(t, ts.URL)
	home.header = map[string]string{"X-Forwarded-For": "192.168.1.20"}
	var info authInfo
	home.do("GET", "/api/auth", nil, &info)
	if info.RemoteFirstRun {
		t.Fatal("home network visitor told to use the setup code")
	}
	if c := home.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim"}, nil); c != 200 {
		t.Fatalf("home welcome = %d", c)
	}
}
