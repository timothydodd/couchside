package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// A link planted in a media folder must not hand out the file it points at
// (the server's auth.key, its database): direct play, HLS and the track list
// answer as if the file were missing. A flagged file isn't streamed either.
func TestMediaRoutesRefusePlantedLinks(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	root := filepath.Join(dir, "media")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	const secret = "the server's signing key"
	if err := os.WriteFile(filepath.Join(dir, "auth.key"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	planted := filepath.Join(root, "Planted (2001).mkv")
	if err := os.Symlink(filepath.Join(dir, "auth.key"), planted); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	real := filepath.Join(root, "Heat (1995).mkv")
	if err := os.WriteFile(real, []byte("video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(root, "Broken (1999).mkv")
	if err := os.WriteFile(broken, []byte("??"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib, err := d.CreateLibrary(ctx, "Films", root, "movies")
	if err != nil {
		t.Fatal(err)
	}
	add := func(path, title, problem string) int64 {
		item, _, err := d.EnsureItem(ctx, lib, "movie", title, 2000)
		if err != nil {
			t.Fatal(err)
		}
		id, err := d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: path, Size: 1, Mtime: 1, Problem: problem}, 1)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	plantedID := add(planted, "Planted", "")
	realID := add(real, "Heat", "")
	brokenID := add(broken, "Broken", "unreadable")

	s, err := New(d, config.Config{DataDir: dir, FFprobe: filepath.Join(dir, "no-ffprobe")}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	web := newClient(t, ts.URL)
	if c := web.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim"}, nil); c != 200 {
		t.Fatalf("welcome = %d", c)
	}
	get := func(id int64) (int, string) {
		res, err := web.http.Get(fmt.Sprintf("%s/api/files/%d/stream", ts.URL, id))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if c, body := get(plantedID); c != http.StatusNotFound || strings.Contains(body, secret) {
		t.Errorf("planted link: %d %q, want 404 without the secret", c, body)
	}
	if c, body := get(realID); c != http.StatusOK || body != "video bytes" {
		t.Errorf("real file: %d %q", c, body)
	}
	if c, _ := get(brokenID); c != http.StatusUnprocessableEntity {
		t.Errorf("flagged file: %d, want 422", c)
	}
	if c := web.do("POST", fmt.Sprintf("/api/files/%d/hls", plantedID), map[string]any{}, nil); c != http.StatusNotFound {
		t.Errorf("hls of a planted link = %d, want 404", c)
	}
	if c := web.do("GET", fmt.Sprintf("/api/files/%d/streams", plantedID), nil, nil); c != http.StatusNotFound {
		t.Errorf("tracks of a planted link = %d, want 404", c)
	}
}
