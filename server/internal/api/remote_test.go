package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

func TestRemoteAllowed(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://image.tmdb.org/t/p/w185/a.jpg":      true,
		"https://m.media-amazon.com/images/M/x.jpg":  true,
		"https://img.hdhomerun.com/channels/US1.png": true,
		"https://evil.com/x.png":                     false,
		"https://image.tmdb.org.evil.com/x.png":      false,
		"https://notmedia-amazon.com/x.png":          false,
		"file:///etc/passwd":                         false,
		"http://169.254.169.254/latest/meta-data/":   false,
	} {
		u, _ := url.Parse(raw)
		if got := remoteAllowed(u); got != want {
			t.Errorf("remoteAllowed(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestRemoteFetchedOnce(t *testing.T) {
	hits := 0
	png := []byte("\x89PNG\r\n\x1a\n0000")
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	}))
	defer origin.Close()
	s := &Server{cfg: config.Config{CacheDir: t.TempDir()}}
	raw := origin.URL + "/logo.png"
	path := s.remotePath(raw)
	for i := 0; i < 2; i++ {
		if _, err := os.Stat(path); err != nil {
			if err := s.fetchRemote(context.Background(), raw, path); err != nil {
				t.Fatal(err)
			}
		}
	}
	if b, _ := os.ReadFile(path); string(b) != string(png) || hits != 1 {
		t.Fatalf("cached %q after %d fetches", b, hits)
	}
	// The handler refuses hosts it doesn't fetch from.
	rec := httptest.NewRecorder()
	s.remoteImage(rec, httptest.NewRequest("GET", "/api/artwork/remote?u="+url.QueryEscape(raw), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("local origin served: %d", rec.Code)
	}
}

func TestImageURLGoesThroughTheCache(t *testing.T) {
	if got := db.CachedImage("https://img.hdhomerun.com/a.png"); got != "/api/artwork/remote?u=https%3A%2F%2Fimg.hdhomerun.com%2Fa.png" {
		t.Errorf("CachedImage = %q", got)
	}
	if db.CachedImage("") != "" {
		t.Error("empty should stay empty")
	}
}
