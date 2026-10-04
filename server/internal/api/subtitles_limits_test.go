//go:build !windows

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// subsServer serves subtitleVTT for one 100-second file, Movie.mkv, with
// ffmpeg as given.
func subsServer(t *testing.T, ffmpeg string) (*Server, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	media := filepath.Join(dir, "media")
	os.MkdirAll(media, 0o755)
	video := filepath.Join(media, "Movie.mkv")
	os.WriteFile(video, []byte("x"), 0o644)
	lib, _ := d.CreateLibrary(ctx, "Films", media, "movies")
	item, _, _ := d.EnsureItem(ctx, lib, "movie", "Movie", 0)
	dur := 100.0
	if _, err := d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: video, Size: 1, Mtime: 1, DurationSec: &dur}, 1); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: d, cfg: config.Config{CacheDir: filepath.Join(dir, "cache"), FFmpeg: ffmpeg}}
	r := chi.NewRouter()
	r.Get("/api/files/{id}/subtitles/{key}", s.subtitleVTT)
	return s, r, media
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestSubtitleChunkPastTheEnd(t *testing.T) {
	s, h, _ := subsServer(t, "/nonexistent/ffmpeg")
	if rec := get(h, "/api/files/1/subtitles/s0.c999999.vtt"); rec.Code != http.StatusNotFound {
		t.Fatalf("chunk past the end = %d, want 404", rec.Code)
	}
	if entries, _ := os.ReadDir(filepath.Join(s.cfg.CacheDir, "subs")); len(entries) != 0 {
		t.Fatalf("a chunk past the end left %d cache files", len(entries))
	}
}

func TestSubtitleErrorsStayInTheLog(t *testing.T) {
	ff := filepath.Join(t.TempDir(), "ffmpeg")
	os.WriteFile(ff, []byte("#!/bin/sh\necho '/srv/private/media/Movie.mkv: Invalid data' >&2\nexit 1\n"), 0o755)
	_, h, _ := subsServer(t, ff)
	rec := get(h, "/api/files/1/subtitles/s0.c0.vtt")
	if rec.Code != http.StatusUnprocessableEntity || strings.Contains(rec.Body.String(), "/srv/private") {
		t.Fatalf("failed conversion = %d %s", rec.Code, rec.Body)
	}
}

func TestSubtitleConversionsWaitForASlot(t *testing.T) {
	dir := t.TempDir()
	ff, log := filepath.Join(dir, "ffmpeg"), filepath.Join(dir, "ran")
	os.WriteFile(ff, []byte("#!/bin/sh\necho ran >> '"+log+"'\nexit 1\n"), 0o755)
	_, h, _ := subsServer(t, ff)
	for i := 0; i < cap(subSlots); i++ {
		subSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(subSlots); i++ {
			<-subSlots
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/files/1/subtitles/s0.c0.vtt", nil).WithContext(ctx))
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a conversion ran with every slot taken")
	}
}

func TestSidecarCacheFollowsTheSidecar(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	_, h, media := subsServer(t, ff)
	srt := filepath.Join(media, "Movie.en.srt")
	write := func(text string) {
		os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:02,000\n"+text+"\n"), 0o644)
	}
	write("first version")
	if rec := get(h, "/api/files/1/subtitles/x0.vtt"); !strings.Contains(rec.Body.String(), "first version") {
		t.Fatalf("first fetch = %d %q", rec.Code, rec.Body)
	}
	write("edited, and longer than before")
	if rec := get(h, "/api/files/1/subtitles/x0.vtt"); !strings.Contains(rec.Body.String(), "edited") {
		t.Fatalf("after editing the sidecar = %q, want the new cues", rec.Body)
	}
}

func TestEmbeddedSubtitleChunkLoads(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	_, h, media := subsServer(t, ff)
	srt := filepath.Join(t.TempDir(), "in.srt")
	os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:02,000\nembedded cue\n"), 0o644)
	out, err := exec.Command(ff, "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=160x120:rate=10", "-i", srt,
		"-t", "3", "-c:v", "libx264", "-c:s", "srt", filepath.Join(media, "Movie.mkv")).CombinedOutput()
	if err != nil {
		t.Fatalf("making a test file: %v %s", err, out)
	}
	if rec := get(h, "/api/files/1/subtitles/s0.c0.vtt"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "embedded cue") {
		t.Fatalf("embedded chunk = %d %q", rec.Code, rec.Body)
	}
}

// A whole embedded track asked for with ?async=1 answers 202 while it's
// converted in the background, then the track; a conversion that fails is
// reported, not retried on every poll.
func TestWholeTrackInTheBackground(t *testing.T) {
	dir := t.TempDir()
	ff := filepath.Join(dir, "ffmpeg")
	runs := filepath.Join(dir, "runs")
	// Takes half a second, then writes the WebVTT file (its last argument).
	script := "#!/bin/sh\necho run >> " + runs + "\nsleep 0.5\nfor a in \"$@\"; do out=\"$a\"; done\nprintf 'WEBVTT\\n\\n00:01.000 --> 00:02.000\\nwhole track\\n' > \"$out\"\n"
	if err := os.WriteFile(ff, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	_, h, _ := subsServer(t, ff)
	// ?prepare=1 starts it too, and only says whether it's ready.
	if rec := get(h, "/api/files/1/subtitles/s0.vtt?prepare=1"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ready":false`) {
		t.Fatalf("prepare while converting = %d %s", rec.Code, rec.Body)
	}
	url := "/api/files/1/subtitles/s0.vtt?async=1"
	for i := 0; i < 3; i++ { // asking again while it runs starts nothing new
		if rec := get(h, url); rec.Code != http.StatusAccepted || rec.Header().Get("Retry-After") == "" {
			t.Fatalf("poll %d while converting = %d", i, rec.Code)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec := get(h, url)
		if rec.Code == 200 {
			if !strings.Contains(rec.Body.String(), "whole track") {
				t.Fatalf("track = %q", rec.Body)
			}
			break
		}
		if rec.Code != http.StatusAccepted || time.Now().After(deadline) {
			t.Fatalf("waiting for the track: %d", rec.Code)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if b, _ := os.ReadFile(runs); strings.Count(string(b), "run") != 1 {
		t.Fatalf("ffmpeg ran %d times for one track", strings.Count(string(b), "run"))
	}
	if rec := get(h, "/api/files/1/subtitles/s0.vtt?prepare=1"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ready":true`) {
		t.Fatalf("prepare once converted = %d %s", rec.Code, rec.Body)
	}
	// Without async, the same URL waits and answers as before (now from the cache).
	if rec := get(h, "/api/files/1/subtitles/s0.vtt"); rec.Code != 200 {
		t.Fatalf("synchronous = %d", rec.Code)
	}

	// A track ffmpeg can't convert: 202 first, then 422 on later polls.
	os.WriteFile(ff, []byte("#!/bin/sh\necho run >> "+runs+"\necho 'Subtitle codec 94213 is not supported' >&2\nexit 1\n"), 0o755)
	bad := "/api/files/1/subtitles/s1.vtt?async=1"
	if rec := get(h, bad); rec.Code != http.StatusAccepted {
		t.Fatalf("first ask for a bad track = %d", rec.Code)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		rec := get(h, bad)
		if rec.Code == http.StatusUnprocessableEntity {
			if strings.Contains(rec.Body.String(), "94213") {
				t.Fatalf("ffmpeg's text reached the client: %s", rec.Body)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a failed conversion keeps answering %d", rec.Code)
		}
		time.Sleep(50 * time.Millisecond)
	}
	before, _ := os.ReadFile(runs)
	get(h, bad)
	get(h, bad)
	time.Sleep(100 * time.Millisecond)
	if after, _ := os.ReadFile(runs); len(after) != len(before) {
		t.Fatal("a failed track was converted again on the next poll")
	}
}
