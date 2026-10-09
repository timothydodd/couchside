//go:build !windows

package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/transcode"
)

// A playback session is the profile's that made it: another profile gets
// "not found" for its playlist and can't close it; an admin can.
func TestHLSSessionBelongsToItsProfile(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	probe := filepath.Join(dir, "ffprobe")
	os.WriteFile(probe, []byte("#!/bin/sh\necho '{\"format\":{\"duration\":\"60\"},\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"hevc\",\"width\":1280,\"height\":720,\"pix_fmt\":\"yuv420p\"}]}'\n"), 0o755)
	tc, err := transcode.NewManager(transcode.Encoder{FFmpeg: filepath.Join(dir, "no-ffmpeg")}, probe, filepath.Join(dir, "hls"), 4)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "media")
	os.MkdirAll(root, 0o755)
	video := filepath.Join(root, "Heat (1995).mkv")
	os.WriteFile(video, []byte("x"), 0o644)
	lib, _ := d.CreateLibrary(ctx, "Films", root, "movies")
	item, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat", 1995)
	dur := 60.0
	fileID, _ := d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: video, Size: 1, Mtime: 1, DurationSec: &dur}, 1)

	s, err := New(d, config.Config{DataDir: dir}, nil, nil, tc, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	if c := admin.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim"}, nil); c != 200 {
		t.Fatalf("welcome = %d", c)
	}
	var kid struct{ ID int64 }
	if c := admin.do("POST", "/api/accounts", map[string]any{"name": "Kid"}, &kid); c != 201 {
		t.Fatalf("create = %d", c)
	}
	owner := newClient(t, ts.URL)
	if c := owner.do("POST", "/api/auth/pick", map[string]any{"profileId": kid.ID}, nil); c != 200 {
		t.Fatalf("pick = %d", c)
	}
	var created struct{ SessionID, Playlist string }
	if c := owner.do("POST", fmt.Sprintf("/api/files/%d/hls", fileID), map[string]any{}, &created); c != 201 {
		t.Fatalf("create hls = %d", c)
	}
	if c := owner.do("GET", created.Playlist, nil, nil); c != 200 {
		t.Fatalf("owner's playlist = %d", c)
	}
	stranger := newClient(t, ts.URL)
	var guest struct{ ID int64 }
	if c := admin.do("POST", "/api/accounts", map[string]any{"name": "Guest"}, &guest); c != 201 {
		t.Fatalf("create guest = %d", c)
	}
	if c := stranger.do("POST", "/api/auth/pick", map[string]any{"profileId": guest.ID}, nil); c != 200 {
		t.Fatalf("pick guest = %d", c)
	}
	if c := stranger.do("GET", created.Playlist, nil, nil); c != 404 {
		t.Fatalf("another profile's playlist = %d, want 404", c)
	}
	stranger.do("DELETE", "/api/hls/"+created.SessionID, nil, nil)
	if tc.Get(created.SessionID) == nil {
		t.Fatal("another profile closed the session")
	}
	if c := admin.do("GET", created.Playlist, nil, nil); c != 200 {
		t.Fatalf("admin's view of the playlist = %d", c)
	}
}
