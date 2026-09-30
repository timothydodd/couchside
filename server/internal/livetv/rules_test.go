package livetv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/transcode"
)

// newTestService builds a Service on a temp database with one library show
// ("Two and a Half Men", owning S11E04) and a day of guide airings.
func newTestService(t *testing.T) (*Service, *db.DB, int64) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	s, err := New(Config{RecordingsDir: filepath.Join(dir, "rec"), PadBefore: time.Minute, PadAfter: 2 * time.Minute},
		d, transcode.Encoder{FFmpeg: "ffmpeg"}, nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ReplaceChannels(ctx, []db.Channel{
		{Number: "2.1", Name: "WSB", URL: "http://x/2.1"},
		{Number: "5.1", Name: "WAGA", URL: "http://x/5.1"},
		{Number: "102.1", Name: "WSB 3.0", URL: "http://x/102.1", DRM: true},
	}, channelSortKey); err != nil {
		t.Fatal(err)
	}
	libID, _ := d.CreateLibrary(ctx, "TV", dir, "tv")
	itemID, _, _ := d.EnsureItem(ctx, libID, "series", "Two and a Half Men", 2003)
	epID, _ := d.EnsureEpisode(ctx, itemID, 11, 4, "", "")
	if _, err := d.UpsertFile(ctx, db.File{LibraryID: libID, MediaItemID: itemID, EpisodeID: &epID, Path: dir + "/s11e04.mkv", Size: 1, Mtime: 1}, 1); err != nil {
		t.Fatal(err)
	}

	h := time.Now().Add(time.Hour).Truncate(time.Hour).Unix()
	prog := func(ch string, at int64, ep, title string, isNew bool) db.Program {
		return db.Program{Channel: ch, StartAt: at, EndAt: at + 1800, Title: "Two and a Half Men", EpisodeNum: ep,
			EpisodeTitle: title, SeriesID: "SH123", IsNew: isNew}
	}
	if err := d.UpsertPrograms(ctx, []db.Program{
		prog("2.1", h, "S11E04", "Clank", false),            // already in library
		prog("2.1", h+1800, "S11E05", "Alan Harper", false), // missing → record
		prog("5.1", h+3600, "S11E05", "Alan Harper", false), // same episode again → duplicate
		prog("2.1", h+7200, "S12E01", "Finale", true),       // new → record
		prog("2.1", h+9000, "", "", false),                  // no episode info, rerun → skip
		prog("102.1", h+10800, "S11E06", "Justice", false),  // DRM channel
		prog("5.1", h+12600, "S11E07", "Seven", false),      // other channel test
	}); err != nil {
		t.Fatal(err)
	}
	return s, d, itemID
}

func firstProgram(t *testing.T, d *db.DB) int64 {
	progs, err := d.UpcomingSeriesPrograms(context.Background(), "SH123", 0)
	if err != nil || len(progs) == 0 {
		t.Fatal("no programs", err)
	}
	return progs[0].ID
}

func TestRuleMissingEpisodes(t *testing.T) {
	s, d, itemID := newTestService(t)
	ctx := context.Background()
	rule, sum, err := s.CreateRule(ctx, firstProgram(t, d), "missing", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rule.MediaItemID == nil || *rule.MediaItemID != itemID {
		t.Errorf("rule should auto-link the library show, got %v", rule.MediaItemID)
	}
	want := RuleSummary{Airings: 7, Scheduled: 3, Added: 3, AlreadyHave: 1, Duplicates: 1, NoEpisodeInfo: 1, Locked: 1}
	if sum != want {
		t.Errorf("summary\n got %+v\nwant %+v", sum, want)
	}
	recs, _ := d.Recordings(ctx)
	eps := map[string]bool{}
	for _, r := range recs {
		eps[r.EpisodeNum] = true
		if r.RuleID == nil || *r.RuleID != rule.ID {
			t.Errorf("recording %s not owned by the rule", r.EpisodeNum)
		}
	}
	for _, e := range []string{"S11E05", "S12E01", "S11E07"} {
		if !eps[e] {
			t.Errorf("expected %s to be scheduled, got %v", e, eps)
		}
	}

	// Re-running is idempotent.
	_, sum2, _ := s.reevaluate(ctx, rule.ID)
	if sum2.Scheduled != 3 || sum2.Added != 0 {
		t.Errorf("second run: %+v", sum2)
	}

	// A cancelled airing is never revived; the rule schedules the episode's other airing instead.
	for _, r := range recs {
		if r.EpisodeNum == "S11E05" {
			_ = d.CancelRecording(ctx, r.ID)
		}
	}
	_, sum3, _ := s.reevaluate(ctx, rule.ID)
	if sum3.Cancelled != 1 || sum3.Scheduled != 3 {
		t.Errorf("after cancel: %+v", sum3)
	}
}

func TestRuleModesAndChannel(t *testing.T) {
	s, d, _ := newTestService(t)
	ctx := context.Background()
	_, sum, err := s.CreateRule(ctx, firstProgram(t, d), "new", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scheduled != 1 || sum.NotNew != 5 {
		t.Errorf("new mode: %+v", sum)
	}
	rules, _ := d.Rules(ctx)
	r := rules[0]
	r.Mode, r.Channel = "all", "2.1"
	_, sum, err = s.UpdateRule(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scheduled != 4 || sum.OtherChannel != 2 {
		t.Errorf("all on 2.1: %+v", sum)
	}
	// Deleting the rule drops its upcoming recordings.
	if err := s.DeleteRule(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if recs, _ := d.Recordings(ctx); len(recs) != 0 {
		t.Errorf("upcoming recordings should be gone, got %d", len(recs))
	}
}

func TestRuleLinksLibraryShowByEpisodeTitles(t *testing.T) {
	s, d, _ := newTestService(t)
	ctx := context.Background()
	libs, _ := d.Libraries(ctx)
	lib := libs[0].ID
	// An older, same-named show that has none of the airing episode titles...
	oldID, _, _ := d.EnsureItem(ctx, lib, "series", "Two and a Half Men", 1990)
	oe, _ := d.EnsureEpisode(ctx, oldID, 1, 1, "Pilot", "")
	_, _ = d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: oldID, EpisodeID: &oe, Path: "/x/old.mkv", Size: 1, Mtime: 1}, 1)
	// ...and the real one, which has "Finale" (airing as S12E01).
	items, _ := d.Items(ctx, "series")
	var realID int64
	for _, it := range items {
		if it.Year != nil && *it.Year == 2003 {
			realID = it.ID
		}
	}
	re, _ := d.EnsureEpisode(ctx, realID, 12, 1, "Finale", "")
	_, _ = d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: realID, EpisodeID: &re, Path: "/x/finale.mkv", Size: 1, Mtime: 1}, 1)

	p, _ := d.Program(ctx, firstProgram(t, d))
	c := s.LibraryMatchesFor(ctx, p)
	if len(c) != 2 || c[0].ID != realID || c[0].Matching != 1 {
		t.Fatalf("want the 2003 show first with 1 matching title, got %+v", c)
	}
}

func TestExistingPartsOrder(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "Show - S01E01.ts")
	for _, n := range []string{final, final[:len(final)-3] + ".part10.ts", final[:len(final)-3] + ".part2.ts", final[:len(final)-3] + ".part1.ts"} {
		if err := os.WriteFile(n, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := existingParts(final)
	want := []string{final, final[:len(final)-3] + ".part1.ts", final[:len(final)-3] + ".part2.ts", final[:len(final)-3] + ".part10.ts"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d: got %s want %s", i, filepath.Base(got[i]), filepath.Base(want[i]))
		}
	}
}

func TestRecordingJoinsExistingShowFolder(t *testing.T) {
	s, _, _ := newTestService(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "The Simpsons (1989)", "Season 07"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 30, 20, 0, 0, 0, time.Local).Unix()
	got := s.pathInDir(dir, recordingName(db.Recording{Title: "The Simpsons", EpisodeNum: "S07E05", EpisodeTitle: "Bart Sells His Soul", StartAt: start}))
	want := filepath.Join(dir, "The Simpsons (1989)", "Season 07", "The Simpsons - S07E05 - Bart Sells His Soul.ts")
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	// A show with no folder yet gets a fresh one.
	got = s.pathInDir(dir, recordingName(db.Recording{Title: "Ghosts", StartAt: start}))
	if filepath.Dir(filepath.Dir(got)) != filepath.Join(dir, "Ghosts") || filepath.Base(filepath.Dir(got)) != "Season 2026" {
		t.Errorf("new show path: %s", got)
	}
}
