package livetv

import (
	"context"
	"github.com/timothydodd/couchside/internal/probe"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/db"
)

func vfile(id, item int64, kind, title string, year int, genres []string, secs float64) db.VirtualFile {
	return db.VirtualFile{FileID: id, ItemID: item, Kind: kind, Title: title, SortTitle: title, Year: year, Genres: genres,
		Path: "/m/" + title, DurationSec: secs, HasAudio: true, Role: "copy", Height: 720}
}

func TestProgramsForGroupsAndFilters(t *testing.T) {
	ep := func(id int64, season, episode int, height int) db.VirtualFile {
		f := vfile(id, 10, "series", "Show", 1962, []string{"Comedy"}, 25*60)
		f.Season, f.Episode, f.Height = season, episode, height
		return f
	}
	part := func(id int64, n int) db.VirtualFile {
		f := vfile(id, 20, "movie", "Long Movie", 1999, []string{"Drama"}, 60*60)
		f.Role, f.PartNo = "part", n
		return f
	}
	files := []db.VirtualFile{
		ep(1, 1, 1, 480), ep(2, 1, 1, 1080), // two copies: the 1080p one wins
		ep(3, 1, 2, 480),
		part(5, 2), part(4, 1), // a movie in two parts, out of order
		vfile(6, 30, "movie", "Action Film", 1988, []string{"Action"}, 100*60),
		vfile(7, 40, "movie", "Tiny", 2000, nil, 5), // too short to schedule
	}
	all := programsFor(files, VirtualConfig{})
	if len(all) != 4 {
		t.Fatalf("want 4 programs (2 episodes, 2 movies), got %d", len(all))
	}
	if all[0].key != "e10:1:1" || all[0].files[0].FileID != 2 {
		t.Errorf("first program should be S1E1 from file 2, got %s from %d", all[0].key, all[0].files[0].FileID)
	}
	for _, p := range all {
		if p.key == "m20" {
			if len(p.files) != 2 || p.files[0].FileID != 4 || p.ms != 2*60*60*1000 {
				t.Errorf("split movie: want parts 4 then 5 lasting 2h, got %+v (%d ms)", p.files, p.ms)
			}
		}
	}
	if got := programsFor(files, VirtualConfig{Kinds: []string{"movie"}, Genres: []string{"action"}}); len(got) != 1 || got[0].key != "m30" {
		t.Errorf("genre filter: got %v", got)
	}
	if got := programsFor(files, VirtualConfig{YearFrom: 1980, YearTo: 1989}); len(got) != 1 || got[0].key != "m30" {
		t.Errorf("year filter: got %v", got)
	}
	if got := programsFor(files, VirtualConfig{ExcludeGenres: []string{"Comedy"}}); len(got) != 2 {
		t.Errorf("exclude filter: got %d programs", len(got))
	}
}

func TestScheduleBreaksAndAlignment(t *testing.T) {
	pool := &fillerPool{rng: rand.New(rand.NewSource(1)), clips: []db.FillerClip{
		{Path: "/ads/a.mp4", DurationMs: 30_000}, {Path: "/ads/b.mp4", DurationMs: 30_000},
	}}
	cfg := VirtualConfig{Filler: Filler{Folder: "/ads", Align: 30, BreakEvery: 8, BreakLength: 60}}
	b := scheduleBuilder{cfg: cfg, pool: pool, number: "900"}
	prog := programsFor([]db.VirtualFile{vfile(1, 1, "movie", "Short", 1950, nil, 22*60)}, VirtualConfig{})[0]

	t0 := time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC).UnixMilli()
	next := b.add(t0, prog)
	if next != t0+30*60_000 {
		t.Fatalf("a 22 minute program with breaks should be padded to the half hour, next starts at +%ds", (next-t0)/1000)
	}
	var showMs, ads int64
	prev := t0
	for _, p := range b.pieces {
		if p.StartMs != prev {
			t.Fatalf("pieces must be back to back: %d then %d", prev, p.StartMs)
		}
		prev = p.EndMs
		if p.Filler {
			ads++
		} else {
			showMs += p.EndMs - p.StartMs
		}
		if p.ProgramAt != t0/1000 {
			t.Errorf("piece belongs to program %d, want %d", p.ProgramAt, t0/1000)
		}
	}
	if showMs != 22*60_000 {
		t.Errorf("the whole program plays: %d ms", showMs)
	}
	// Breaks at 8 and 16 minutes (2 clips each), and 6 minutes of padding (12 clips).
	if ads != 16 {
		t.Errorf("want 16 ad clips, got %d", ads)
	}
	if len(b.guide) != 1 || b.guide[0].StartAt != t0/1000 || b.guide[0].EndAt != next/1000 {
		t.Errorf("the guide program covers its commercials: %+v", b.guide)
	}
}

func TestSequentialRotatesShows(t *testing.T) {
	var files []db.VirtualFile
	for i, show := range []string{"Alpha", "Beta"} {
		for e := 1; e <= 2; e++ {
			f := vfile(int64(i*10+e), int64(i+1), "series", show, 1960, nil, 25*60)
			f.Season, f.Episode = 1, e
			files = append(files, f)
		}
	}
	p := newPicker("sequential", programsFor(files, VirtualConfig{}), &virtualState{}, rand.New(rand.NewSource(1)))
	var got []string
	for range 5 {
		got = append(got, p.next().key)
	}
	want := []string{"e1:1:1", "e2:1:1", "e1:1:2", "e2:1:2", "e1:1:1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sequential order: got %v, want %v", got, want)
		}
	}
}

func TestVirtualChannelLifecycle(t *testing.T) {
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
	// A commercials folder whose clip lengths are already known (no probing).
	ads := t.TempDir()
	clip := filepath.Join(ads, "ad.mp4")
	if err := os.WriteFile(clip, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(clip)
	_ = d.SaveFillerClip(ctx, db.FillerClip{Path: clip, Mtime: st.ModTime().Unix(), DurationMs: 30_000, HasAudio: true})

	cfg := VirtualConfig{Libraries: []int64{lib}, Filler: Filler{Folder: ads, Align: 30}}
	id, err := s.SaveVirtual(ctx, 0, "900", "Movie Night", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if e := s.VirtualError(id); e != "" {
		t.Fatalf("schedule error: %s", e)
	}
	if _, err := s.SaveVirtual(ctx, 0, "2.1", "Clash", cfg); err == nil {
		t.Error("a tuner channel's number can't be reused")
	}

	ch, err := d.Channel(ctx, "900")
	if err != nil || !ch.Virtual || ch.Name != "Movie Night" {
		t.Fatalf("lineup row: %+v %v", ch, err)
	}
	now := time.Now()
	pieces, err := d.PlayoutFrom(ctx, id, now.UnixMilli(), 4)
	if err != nil || len(pieces) == 0 || pieces[0].StartMs > now.UnixMilli() {
		t.Fatalf("something is on now: %+v %v", pieces, err)
	}
	if end, _ := d.PlayoutEnd(ctx, id); end < now.Add(virtualAhead-time.Hour).UnixMilli() {
		t.Errorf("schedule should run about %v ahead", virtualAhead)
	}
	on, err := d.ProgramAt(ctx, "900", now.Unix())
	if err != nil || on == nil {
		t.Fatalf("the guide has what's on: %v", err)
	}
	if on.EpisodeTitle == "" || on.Categories[0] != "Movie" {
		t.Errorf("movie guide entry: %+v", on)
	}

	// Renumbering moves the channel and rebuilds its guide.
	if _, err := s.SaveVirtual(ctx, id, "901", "Movie Night", cfg); err != nil {
		t.Fatal(err)
	}
	if p, _ := d.ProgramAt(ctx, "900", now.Unix()); p != nil {
		t.Error("the old number's guide should be gone")
	}
	if p, _ := d.ProgramAt(ctx, "901", now.Unix()); p == nil {
		t.Error("the new number has a guide")
	}
	// A lineup refresh from the tuner leaves virtual channels alone.
	if err := d.ReplaceChannels(ctx, []db.Channel{{Number: "2.1", Name: "WSB", URL: "http://x/2.1"}}, channelSortKey); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Channel(ctx, "901"); err != nil {
		t.Error("virtual channel was removed by a tuner refresh")
	}
	if err := s.DeleteVirtual(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Channel(ctx, "901"); err == nil {
		t.Error("deleted channel still in the lineup")
	}
	if p, _ := d.ProgramAt(ctx, "901", now.Unix()); p != nil {
		t.Error("deleted channel still in the guide")
	}
}

func TestNothingToPlay(t *testing.T) {
	s, _, _ := newTestService(t)
	id, err := s.SaveVirtual(context.Background(), 0, "950", "Empty", VirtualConfig{Genres: []string{"Nope"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.VirtualError(id) != ErrNothingToPlay.Error() {
		t.Errorf("want %q, got %q", ErrNothingToPlay, s.VirtualError(id))
	}
}

func TestExtendVirtualConcurrent(t *testing.T) {
	s, d, _ := newTestService(t)
	ctx := context.Background()
	dir := t.TempDir()
	lib, _ := d.CreateLibrary(ctx, "Movies", dir, "movies")
	item, _, _ := d.EnsureItem(ctx, lib, "movie", "Only", 1950)
	dur := 30 * 60.0
	_, _ = d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: filepath.Join(dir, "only.mp4"), Size: 1, Mtime: 1, DurationSec: &dur}, 1)
	id, err := d.CreateVirtualChannel(ctx, "960", "Race", []byte(`{}`), 960)
	if err != nil {
		t.Fatal(err)
	}
	vc, _ := d.VirtualChannel(ctx, id)
	done := make(chan error, 4)
	for range 4 {
		go func() { done <- s.extendVirtual(ctx, vc, time.Now()) }()
	}
	for range 4 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	pieces, _ := d.PlayoutFrom(ctx, id, 0, 1000)
	for i := 1; i < len(pieces); i++ {
		if pieces[i].StartMs != pieces[i-1].EndMs {
			t.Fatalf("schedule overlaps or has gaps at piece %d: %d after %d", i, pieces[i].StartMs, pieces[i-1].EndMs)
		}
	}
}

// A stream channel's address must be http(s): ffmpeg would open anything,
// the server's own files included.
func TestStreamSourceAddress(t *testing.T) {
	for addr, ok := range map[string]bool{
		"http://ws4channels:9798/stream.m3u8": true,
		" https://example.com/live.ts ":       true,
		"file:///etc/passwd":                  false,
		"concat:/data/a.ts|/data/b.ts":        false,
		"/media/Movies/film.mkv":              false,
		"rtsp://camera/stream":                false,
		"http://":                             false,
		"":                                    false,
	} {
		c := VirtualConfig{Stream: &StreamSource{URL: addr}}
		if err := c.Validate(); (err == nil) != ok {
			t.Errorf("%q: err = %v, want ok = %v", addr, err, ok)
		}
	}
}

// A stream channel's guide is its name, hour by hour from the current hour.
func TestStreamGuide(t *testing.T) {
	now := time.Date(2026, 10, 5, 14, 37, 0, 0, time.UTC)
	g := streamGuide("950", "Weather", now, 3*time.Hour)
	if len(g) != 4 || g[0].StartAt != now.Truncate(time.Hour).Unix() || g[0].EndAt != g[1].StartAt || g[3].EndAt < now.Add(3*time.Hour).Unix() {
		t.Fatalf("guide = %+v", g)
	}
	if g[0].Channel != "950" || g[0].Title != "Weather" {
		t.Fatalf("entry = %+v", g[0])
	}
}

// A stream that's already H.264 and AAC is passed through; anything else is
// converted, and an unreadable one too.
func TestStreamSpec(t *testing.T) {
	o := WatchOpts{Height: 720}
	h264 := &probe.Info{VideoCodec: "h264", PixFmt: "yuv420p", AudioCodec: "aac", AudioChannels: 2}
	if sp := streamSpec(h264, o); !sp.CopyVideo || !sp.CopyAudio || sp.VideoCodec != "h264" {
		t.Fatalf("h264+aac: %+v", sp)
	}
	tenBit := &probe.Info{VideoCodec: "h264", PixFmt: "yuv420p10le", AudioCodec: "ac3", AudioChannels: 6}
	if sp := streamSpec(tenBit, o); sp.CopyVideo || sp.CopyAudio {
		t.Fatalf("10-bit + ac3: %+v", sp)
	}
	hevc := &probe.Info{VideoCodec: "hevc", PixFmt: "yuv420p", AudioCodec: "aac", AudioChannels: 2}
	if sp := streamSpec(hevc, o); sp.CopyVideo || !sp.CopyAudio {
		t.Fatalf("hevc+aac: %+v", sp)
	}
	if sp := streamSpec(nil, o); sp.CopyVideo || sp.CopyAudio || sp.Height != 720 {
		t.Fatalf("unknown: %+v", sp)
	}
}
