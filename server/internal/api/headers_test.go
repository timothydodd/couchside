package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
)

func TestSecurityHeaders(t *testing.T) {
	web := t.TempDir()
	os.WriteFile(filepath.Join(web, "index.html"), []byte("<!doctype html>"), 0o644)
	s := &Server{cfg: config.Config{WebDir: web}, presence: newPresence()}
	h := s.Handler()
	for _, path := range []string{"/healthz", "/livez", "/movies"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", path)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/movies", nil))
	if got := rec.Header().Get("Content-Security-Policy"); got != uiCSP {
		t.Errorf("UI CSP = %q", got)
	}
	if rec.Header().Get("Referrer-Policy") != "same-origin" {
		t.Errorf("no Referrer-Policy")
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Errorf("HSTS over plain HTTP")
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Header().Get("Content-Security-Policy") != "" || rec.Header().Get("Referrer-Policy") == "" {
		t.Errorf("/healthz headers = %v", rec.Header())
	}
}

// HSTS only once the browser came over HTTPS: directly, or via a trusted
// proxy's X-Forwarded-Proto, never on an untrusted peer's say-so.
func TestHSTS(t *testing.T) {
	proxies, err := parseProxies([]string{"192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{proxies: proxies, presence: newPresence()}
	h := s.Handler()
	hsts := func(r *http.Request) string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Header().Get("Strict-Transport-Security")
	}
	direct := httptest.NewRequest("GET", "/livez", nil)
	direct.TLS = &tls.ConnectionState{}
	if hsts(direct) != "max-age=31536000" {
		t.Error("no HSTS over direct TLS")
	}
	proxied := httptest.NewRequest("GET", "/livez", nil)
	proxied.RemoteAddr = "192.0.2.1:1234"
	proxied.Header.Set("X-Forwarded-Proto", "https")
	if hsts(proxied) == "" {
		t.Error("no HSTS behind a trusted HTTPS proxy")
	}
	spoofed := httptest.NewRequest("GET", "/livez", nil)
	spoofed.RemoteAddr = "203.0.113.5:1234"
	spoofed.Header.Set("X-Forwarded-Proto", "https")
	if hsts(spoofed) != "" {
		t.Error("HSTS on an untrusted peer's X-Forwarded-Proto")
	}
}

// The liveness probe answers without a database: this server has none, so
// any database use would fail it.
func TestLivezNeedsNoDatabase(t *testing.T) {
	h := (&Server{presence: newPresence()}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/livez", nil))
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("/livez = %d %q", rec.Code, rec.Body.String())
	}
}
