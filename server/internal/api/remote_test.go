package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/remoteimg"
)

var testPNG = []byte("\x89PNG\r\n\x1a\n0000")

// remoteFixture serves images from an httptest origin that the allowlist
// accepts as 127.0.0.1; "localhost" stays off the list.
func remoteFixture(t *testing.T, h http.HandlerFunc) (*Server, *httptest.Server) {
	t.Helper()
	old := remoteimg.Hosts
	remoteimg.Hosts = []remoteimg.Host{{Name: "127.0.0.1", HTTP: true}}
	t.Cleanup(func() { remoteimg.Hosts = old })
	origin := httptest.NewServer(h)
	t.Cleanup(origin.Close)
	return &Server{cfg: config.Config{CacheDir: t.TempDir()}}, origin
}

func getRemote(s *Server, raw string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.remoteImage(rec, httptest.NewRequest("GET", "/api/artwork/remote?u="+url.QueryEscape(raw), nil))
	return rec
}

func TestRemoteFetchedOnceAndServedSafely(t *testing.T) {
	hits := 0
	s, origin := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		w.Write(testPNG)
	})
	// Two URLs differing only by query (and fragment) are one cache file.
	for _, q := range []string{"?x=1", "?x=2#f"} {
		rec := getRemote(s, origin.URL+"/logo.png"+q)
		if rec.Code != 200 || rec.Body.String() != string(testPNG) {
			t.Fatalf("get %s = %d %q", q, rec.Code, rec.Body.String())
		}
		if ct, ns := rec.Header().Get("Content-Type"), rec.Header().Get("X-Content-Type-Options"); ct != "image/png" || ns != "nosniff" {
			t.Fatalf("headers: type %q, nosniff %q", ct, ns)
		}
	}
	if hits != 1 {
		t.Fatalf("origin hit %d times, want 1", hits)
	}
	// Hosts off the allowlist are refused outright.
	if rec := getRemote(s, strings.Replace(origin.URL, "127.0.0.1", "localhost", 1)+"/logo.png"); rec.Code != http.StatusBadRequest {
		t.Fatalf("disallowed host = %d", rec.Code)
	}
}

func TestRemoteRefusesWhatIsNotAnImage(t *testing.T) {
	var redirectTo string
	s, origin := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect.png", "/redirect2.png":
			http.Redirect(w, r, redirectTo, http.StatusFound)
		case "/page.png": // HTML claiming to be a PNG
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("<!DOCTYPE html><html><script>alert(document.cookie)</script></html>"))
		case "/icon.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
		case "/untyped.png":
			w.Write(testPNG[:0])
		default:
			w.Header().Set("Content-Type", "image/png")
			w.Write(testPNG)
		}
	})
	redirectTo = strings.Replace(origin.URL, "127.0.0.1", "localhost", 1) + "/ok.png"
	for _, p := range []string{"/redirect.png", "/page.png", "/icon.svg", "/untyped.png"} {
		raw := origin.URL + p
		if rec := getRemote(s, raw); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", p, rec.Code)
		}
		u, _ := url.Parse(raw)
		if _, err := os.Stat(s.remotePath(remoteimg.Key(u))); err == nil {
			t.Errorf("%s was cached", p)
		}
	}
	// A redirect that stays on the allowlist is followed.
	redirectTo = origin.URL + "/ok.png"
	if rec := getRemote(s, origin.URL+"/redirect2.png"); rec.Code != 200 {
		t.Fatalf("allowed redirect = %d", rec.Code)
	}
}

func TestRemoteMissIsRemembered(t *testing.T) {
	hits := 0
	s, origin := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	})
	for i := 0; i < 3; i++ {
		getRemote(s, origin.URL+"/gone.png")
	}
	if hits != 1 {
		t.Fatalf("a missing image was fetched %d times, want 1", hits)
	}
}

func TestRemoteFetchUsesCanonicalKey(t *testing.T) {
	s, origin := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(testPNG)
	})
	u, _ := url.Parse(origin.URL + "/a.png?junk=1")
	key := remoteimg.Key(u)
	if err := s.fetchRemote(context.Background(), key, s.remotePath(key)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(key, "junk") {
		t.Fatalf("key kept the query: %s", key)
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
