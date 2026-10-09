package api

import (
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
	if rec.Header().Get("Content-Security-Policy") != "frame-ancestors 'self'" {
		t.Errorf("UI CSP = %q", rec.Header().Get("Content-Security-Policy"))
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
