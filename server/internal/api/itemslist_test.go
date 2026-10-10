package api

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// The library list is gzipped when asked, answers 304 while nothing changed,
// and a new ETag once something did. Direct play is never compressed.
func TestItemsListRevalidates(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	root := filepath.Join(dir, "media")
	os.MkdirAll(root, 0o755)
	video := filepath.Join(root, "Heat (1995).mkv")
	os.WriteFile(video, make([]byte, 4096), 0o644)
	lib, _ := d.CreateLibrary(ctx, "Films", root, "movies")
	item, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat", 1995)
	fileID, err := d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: video, Size: 4096, Mtime: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(d, config.Config{DataDir: dir}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	web := newClient(t, ts.URL)
	if c := web.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim"}, nil); c != 200 {
		t.Fatalf("welcome = %d", c)
	}
	// A raw transport, so gzip and 304s are visible as they arrive.
	raw := &http.Client{Jar: web.http.Jar, Transport: &http.Transport{DisableCompression: true}}
	get := func(path, inm string) *http.Response {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		res, err := raw.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := get("/api/items?kind=movie", "")
	tag := res.Header.Get("ETag")
	if res.StatusCode != 200 || tag == "" || res.Header.Get("Content-Encoding") != "gzip" || res.Header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("first = %d etag %q encoding %q cache %q", res.StatusCode, tag, res.Header.Get("Content-Encoding"), res.Header.Get("Cache-Control"))
	}
	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(zr); len(b) < 10 {
		t.Fatalf("body = %q", b)
	}
	res.Body.Close()

	res = get("/api/items?kind=movie", tag)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusNotModified || len(body) != 0 {
		t.Fatalf("unchanged = %d with %d bytes, want 304 and none", res.StatusCode, len(body))
	}
	if c := web.do("POST", fmt.Sprintf("/api/files/%d/watched", fileID), map[string]bool{"watched": true}, nil); c >= 300 {
		t.Fatalf("mark watched = %d", c)
	}
	res = get("/api/items?kind=movie", tag)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("ETag") == tag {
		t.Fatalf("after a change = %d, etag %q (was %q)", res.StatusCode, res.Header.Get("ETag"), tag)
	}

	res = get(fmt.Sprintf("/api/files/%d/stream", fileID), "")
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("stream = %d, encoding %q; want 200 and never gzip", res.StatusCode, res.Header.Get("Content-Encoding"))
	}
}
