package api

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// An admin downloads a diagnostics zip with what a bug report needs, and
// without the API keys in it.
func TestDiagnostics(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, config.Config{DataDir: dir, CacheDir: dir, TMDBKey: "SECRET-TMDB-123", OMDbKey: "SECRET-OMDB-456"}, nil, nil, nil, nil, "0.19.0")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	web := newClient(t, ts.URL)
	if c := web.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim"}, nil); c != 200 {
		t.Fatalf("welcome = %d", c)
	}
	res, err := web.http.Get(ts.URL + "/api/system/diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("diagnostics = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if bytes.Contains(body, []byte("SECRET-")) {
		t.Fatal("an API key is in the zip")
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
		if strings.Contains(string(b), "SECRET-") {
			t.Errorf("%s holds an API key", f.Name)
		}
	}
	for _, want := range []string{"server.json", "config.json", "counts.json", "migrations.txt", "database.txt", "disk.json", "log.txt"} {
		if _, ok := files[want]; !ok {
			t.Errorf("no %s in %v", want, keys(files))
		}
	}
	if !strings.Contains(files["config.json"], `"custom"`) || !strings.Contains(files["config.json"], `"set"`) {
		t.Errorf("config.json doesn't say the keys are set: %s", files["config.json"])
	}
	if !strings.Contains(files["migrations.txt"], "0001_init.sql") || !strings.Contains(files["database.txt"], "quick_check: ok") {
		t.Errorf("migrations or database health missing:\n%s\n%s", files["migrations.txt"], files["database.txt"])
	}
	if !strings.Contains(files["server.json"], `"0.19.0"`) {
		t.Errorf("server.json = %s", files["server.json"])
	}
}

func keys(m map[string]string) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}
