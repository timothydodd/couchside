package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/auth"
	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/usererr"
)

// passwordlessServer starts a server where nobody has a password yet, with
// the admin (profile 1) signed in.
func passwordlessServer(t *testing.T) (*Server, *httptest.Server, *testClient) {
	t.Helper()
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
	admin := newClient(t, ts.URL)
	if code := admin.do("POST", "/api/auth/pick", map[string]any{"profileId": 1}, nil); code != 200 {
		t.Fatalf("admin pick = %d", code)
	}
	return s, ts, admin
}

func TestLoginRefusesOverlongInputAlike(t *testing.T) {
	s, ts, admin := passwordlessServer(t)
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid", "password": "kid password"}, nil); code != 201 {
		t.Fatalf("create = %d", code)
	}
	// Any failure locks a name at once, and addresses never, so each check is visible.
	s.auth.byName = auth.NewLimiter(0, time.Minute, time.Minute, time.Minute)
	s.auth.byIP = auth.NewLimiter(1000, time.Minute, time.Minute, time.Minute)
	c := newClient(t, ts.URL)

	long := strings.Repeat("x", auth.MaxPassword+1)
	var known, unknown map[string]string
	codeK := c.do("POST", "/api/auth/login", map[string]string{"name": "Kid", "password": long}, &known)
	codeU := c.do("POST", "/api/auth/login", map[string]string{"name": "Nobody", "password": long}, &unknown)
	if codeK != 401 || codeU != 401 || known["code"] != "bad_credentials" || known["error"] != unknown["error"] {
		t.Fatalf("over-long password: known %d %v, unknown %d %v", codeK, known, codeU, unknown)
	}
	if s.auth.byName.Wait("kid") == 0 || s.auth.byName.Wait("nobody") == 0 {
		t.Fatal("over-long passwords weren't counted against the name")
	}

	// A name longer than any account's: the same 401, and no limiter key.
	huge := strings.Repeat("n", 10000)
	if code := c.do("POST", "/api/auth/login", map[string]string{"name": huge, "password": "pw"}, nil); code != 401 {
		t.Fatalf("10k-character name = %d", code)
	}
	if s.auth.byName.Wait(huge) != 0 {
		t.Fatal("a 10k-character name became a limiter key")
	}
}

func TestDisableWithClashingRenameSignsOut(t *testing.T) {
	_, ts, admin := passwordlessServer(t)
	var kid db.Profile
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid"}, &kid); code != 201 {
		t.Fatalf("create = %d", code)
	}
	tv := newClient(t, ts.URL)
	var tk tokens
	if code := tv.do("POST", "/api/auth/pick", map[string]any{"profileId": kid.ID, "client": "tv"}, &tk); code != 200 {
		t.Fatalf("kid pick = %d", code)
	}
	tv.bearer = tk.AccessToken
	if code := tv.do("GET", "/api/home", nil, nil); code != 200 {
		t.Fatalf("kid home = %d", code)
	}
	// Disable, and rename to a taken name: the rename fails, the disable holds.
	if code := admin.do("PUT", "/api/accounts/"+strconv.FormatInt(kid.ID, 10), map[string]any{"name": "Me", "role": "user", "disabled": true}, nil); code != 400 {
		t.Fatalf("clashing rename = %d, want 400", code)
	}
	if code := tv.do("GET", "/api/home", nil, nil); code != 401 {
		t.Fatalf("disabled account still signed in: %d", code)
	}
}

func TestPasswordlessOffEndsPasswordlessSessions(t *testing.T) {
	_, ts, admin := passwordlessServer(t)
	var guest db.Profile
	if code := admin.do("POST", "/api/accounts", map[string]any{"name": "Guest"}, &guest); code != 201 {
		t.Fatalf("create = %d", code)
	}
	g := newClient(t, ts.URL)
	var tk tokens
	if code := g.do("POST", "/api/auth/pick", map[string]any{"profileId": guest.ID, "client": "tv"}, &tk); code != 200 {
		t.Fatalf("guest pick = %d", code)
	}
	g.bearer = tk.AccessToken
	if code := admin.do("POST", "/api/auth/password", map[string]string{"current": "", "password": "admin password"}, nil); code != 204 {
		t.Fatalf("admin password = %d", code)
	}
	if code := admin.do("PUT", "/api/settings/passwordless", map[string]bool{"enabled": false}, nil); code != 204 {
		t.Fatalf("passwordless off = %d", code)
	}
	if code := g.do("GET", "/api/home", nil, nil); code != 401 {
		t.Fatalf("passwordless profile still signed in after passwordless went off: %d", code)
	}
	if code := admin.do("GET", "/api/home", nil, nil); code != 200 {
		t.Fatalf("admin (has a password) signed out too: %d", code)
	}
}

func TestPickIsThrottledAndSessionsCapped(t *testing.T) {
	s, ts, _ := passwordlessServer(t)
	c := newClient(t, ts.URL)
	code := 0
	for i := 0; i < 25 && code != 429; i++ {
		code = c.do("POST", "/api/auth/pick", map[string]any{"profileId": 1, "client": "tv"}, nil)
	}
	if code != 429 {
		t.Fatalf("25 picks from one address = %d, want 429", code)
	}

	ctx := context.Background()
	for i := 0; i < maxSessions+5; i++ {
		if err := s.db.CreateSession(ctx, db.Session{ID: "s" + strconv.Itoa(i), ProfileID: 1, Client: "tv", ExpiresAt: time.Now().Add(time.Hour).Unix()}, "h"+strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.db.TrimSessions(ctx, 1, maxSessions); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.db.Sessions(ctx, 1, time.Now().Unix()); len(got) != maxSessions {
		t.Fatalf("sessions after trim = %d, want %d", len(got), maxSessions)
	}
}

func TestInternalErrorsStayInTheLog(t *testing.T) {
	rec := httptest.NewRecorder()
	writeErr(rec, errors.New("open /srv/secret/db: SQL logic error"))
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("internal error leaked: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	writeErr(rec, usererr.New("all tuners are busy"))
	if !strings.Contains(rec.Body.String(), "all tuners are busy") {
		t.Fatalf("user-facing error hidden: %s", rec.Body)
	}
}

// COUCHSIDE_AUTH=true on a server that was passwordless: sessions of profiles
// without a password end when the server starts, not when it stops.
func TestRunEndsPasswordlessSessionsAtStart(t *testing.T) {
	s, _, admin := passwordlessServer(t)
	if code := admin.do("GET", "/api/auth/sessions", nil, nil); code != 200 {
		t.Fatalf("sessions before = %d", code)
	}
	s.cfg.Auth = true
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	deadline := time.Now().Add(5 * time.Second)
	for admin.do("GET", "/api/auth/sessions", nil, nil) != 401 {
		select {
		case <-done:
			t.Fatal("Run returned before ctx ended")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the passwordless session still worked while the server ran")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
