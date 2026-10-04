package livetv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/db"
)

// movieChannel is a service with a three-film library and a virtual channel
// on 900.
func movieChannel(t *testing.T) (*Service, *db.DB, int64, VirtualConfig) {
	t.Helper()
	s, d, _ := newTestService(t)
	ctx := context.Background()
	dir := t.TempDir()
	lib, _ := d.CreateLibrary(ctx, "Movies", dir, "movies")
	for i, title := range []string{"One", "Two", "Three"} {
		item, _, _ := d.EnsureItem(ctx, lib, "movie", title, 1950+i)
		dur := 90 * 60.0
		if _, err := d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: filepath.Join(dir, title+".mp4"),
			Size: 1, Mtime: 1, DurationSec: &dur, AudioCodec: "aac", VideoCodec: "h264"}, 1); err != nil {
			t.Fatal(err)
		}
	}
	cfg := VirtualConfig{Libraries: []int64{lib}}
	id, err := s.SaveVirtual(ctx, 0, "900", "Movie Night", cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s, d, id, cfg
}

// Saving a channel waits for a schedule extension that's under way, so the
// extension can't put the old schedule back after the save cleared it.
func TestSaveVirtualWaitsForTheScheduleBuilder(t *testing.T) {
	s, _, id, cfg := movieChannel(t)
	s.virtualMu.Lock() // an extension is running
	saved := make(chan error, 1)
	go func() {
		_, err := s.SaveVirtual(context.Background(), id, "901", "Movie Night", cfg)
		saved <- err
	}()
	select {
	case err := <-saved:
		t.Fatalf("the save went ahead beside a schedule extension (%v)", err)
	case <-time.After(150 * time.Millisecond):
	}
	s.virtualMu.Unlock()
	select {
	case err := <-saved:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the save never finished")
	}
}

// With no tuner there's no guide refresh, so the schedule loop prunes old
// listings itself.
func TestVirtualGuideIsPrunedWithoutATuner(t *testing.T) {
	s, d, _, _ := movieChannel(t)
	ctx := context.Background()
	old := time.Now().Add(-48 * time.Hour).Unix()
	if err := d.UpsertPrograms(ctx, []db.Program{{Channel: "900", StartAt: old, EndAt: old + 1800, Title: "Long gone"}}); err != nil {
		t.Fatal(err)
	}
	s.extendAllVirtual(ctx)
	if p, _ := d.ProgramAt(ctx, "900", old+60); p != nil {
		t.Fatal("a listing from two days ago is still in the guide")
	}
	if p, _ := d.ProgramAt(ctx, "900", time.Now().Unix()); p == nil {
		t.Fatal("what's on now was pruned")
	}
}

// Renumbering onto a number someone still has as a favourite (a tuner channel
// that left the lineup) used to break the favourites key.
func TestRenumberOntoAStaleFavourite(t *testing.T) {
	s, d, id, cfg := movieChannel(t)
	ctx := context.Background()
	p, err := d.CreateProfile(ctx, "Alice", "accent")
	if err != nil {
		t.Fatal(err)
	}
	alice := db.WithProfile(ctx, p.ID)
	for _, n := range []string{"5.1", "900"} {
		if err := d.SetChannelPinned(alice, n, true); err != nil {
			t.Fatal(err)
		}
	}
	// 5.1 leaves the lineup; the favourite may stay behind.
	if err := d.ReplaceChannels(ctx, []db.Channel{{Number: "2.1", Name: "WSB", URL: "http://x/2.1"}}, channelSortKey); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveVirtual(ctx, id, "5.1", "Movie Night", cfg); err != nil {
		t.Fatalf("renumber onto a stale favourite: %v", err)
	}
	chans, err := d.Channels(alice)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chans {
		if c.Number == "5.1" && !c.Pinned {
			t.Fatal("the favourite didn't follow the channel to its new number")
		}
	}
}

// The merged playlist slides: old segments leave the window (and the disk),
// and discontinuities that left are counted.
func TestMergedPlaylistWindow(t *testing.T) {
	dir := t.TempDir()
	pl := &mergedPlaylist{dir: dir}
	writeRun := func(run, segs int) string {
		var b strings.Builder
		b.WriteString("#EXTM3U\n")
		for i := range segs {
			name := fmt.Sprintf("seg%d_%d.ts", run, i)
			_ = os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644)
			fmt.Fprintf(&b, "#EXTINF:2.000,\n%s\n", name)
		}
		p := filepath.Join(dir, fmt.Sprintf("run%d.m3u8", run))
		_ = os.WriteFile(p, []byte(b.String()), 0o644)
		return p
	}
	pl.follow(writeRun(0, 3), 0)
	pl.follow(writeRun(0, 5), 0) // the run grew: only the new two are added
	if pl.count() != 5 || pl.runMs(0) != 10_000 {
		t.Fatalf("after one run: %d segments, %dms", pl.count(), pl.runMs(0))
	}
	pl.follow(writeRun(1, virtualWindow), 1)
	pl.follow(writeRun(2, 4), 2)
	index, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	if n := strings.Count(text, "#EXTINF"); n != virtualWindow {
		t.Fatalf("%d segments listed, want %d", n, virtualWindow)
	}
	// Run 0 and the start of run 1 have gone: run 1's discontinuity with them.
	if !strings.Contains(text, "#EXT-X-MEDIA-SEQUENCE:9\n") || !strings.Contains(text, "#EXT-X-DISCONTINUITY-SEQUENCE:1\n") {
		t.Fatalf("header:\n%s", text[:200])
	}
	if strings.Count(text, "#EXT-X-DISCONTINUITY\n") != 1 {
		t.Fatal("run 2 should start with the one discontinuity left in the window")
	}
	if _, err := os.Stat(filepath.Join(dir, "seg0_0.ts")); err == nil {
		t.Fatal("a segment that left the window is still on disk")
	}
	if _, err := os.Stat(filepath.Join(dir, "seg2_3.ts")); err != nil {
		t.Fatal("the newest segment is missing")
	}
}
