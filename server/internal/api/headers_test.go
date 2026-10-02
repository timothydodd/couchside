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
	for _, path := range []string{"/healthz", "/movies"} {
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
