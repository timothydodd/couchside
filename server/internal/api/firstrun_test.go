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
