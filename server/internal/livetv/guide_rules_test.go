package livetv

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/timothydodd/couchside/internal/db"
)

// A show moved from 8:00 to 8:15: the next guide page replaces the old row
// instead of leaving two overlapping listings.
func TestGuidePageReplacesMovedPrograms(t *testing.T) {
	_, d, _ := newTestService(t)
	ctx := context.Background()
	at := time.Now().Add(48 * time.Hour).Truncate(time.Hour).Unix()
	page := func(start int64) []db.Program {
		return []db.Program{
			{Channel: "5.1", StartAt: at - 1800, EndAt: start, Title: "Before"},
			{Channel: "5.1", StartAt: start, EndAt: at + 3600, Title: "The Show"},
		}
	}
	if err := d.UpsertPrograms(ctx, page(at)); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertPrograms(ctx, page(at+900)); err != nil {
		t.Fatal(err)
	}
	if p, _ := d.ProgramAt(ctx, "5.1", at+60); p == nil || p.Title != "Before" {
		t.Fatalf("at 8:01 the guide shows %+v, want the program before (the show moved to 8:15)", p)
	}
	// Another channel's listings in the same hours are untouched.
	if p, _ := d.ProgramAt(ctx, "2.1", time.Now().Add(2*time.Hour).Truncate(time.Hour).Unix()+60); p == nil {
		t.Fatal("another channel's program was deleted")
	}
}

func TestPeakOverlap(t *testing.T) {
	h := int64(3600)
	wins := [][2]int64{{7 * h, 8*h + 300}, {8*h + 3300, 10 * h}} // 7:00–8:05 and 8:55–10:00
	if got := peakOverlap(wins, 8*h, 9*h); got != 1 {
		t.Errorf("8:00–9:00 next to two recordings that never overlap: peak %d, want 1", got)
	}
	wins = append(wins, [2]int64{8*h + 1800, 9*h + 1800}) // 8:30–9:30 overlaps the 8:55 one
	if got := peakOverlap(wins, 8*h, 9*h); got != 2 {
		t.Errorf("peak %d, want 2", got)
	}
	if got := peakOverlap([][2]int64{{0, 10}, {10, 20}}, 0, 20); got != 1 {
		t.Errorf("back-to-back recordings: peak %d, want 1", got)
	}
}

// "New" mode skips an episode already recorded even if a later airing is
// still flagged new.
func TestNewModeSkipsRecordedEpisodes(t *testing.T) {
	s, d, _ := newTestService(t)
	ctx := context.Background()
	past := time.Now().Add(-48 * time.Hour).Unix()
	id, _, err := d.ScheduleRecording(ctx, db.Recording{Channel: "5.1", Title: "Two and a Half Men", EpisodeNum: "S12E01",
		EpisodeTitle: "Finale", SeriesID: "SH123", StartAt: past, EndAt: past + 1800}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishRecording(ctx, id, "completed", "/rec/finale.ts", 1, ""); err != nil {
		t.Fatal(err)
	}
	_, sum, err := s.CreateRule(ctx, firstProgram(t, d), "new", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scheduled != 0 || sum.AlreadyRecorded != 1 {
		t.Fatalf("new mode with S12E01 already recorded: %+v", sum)
	}
}

// A rule's failed airing that's still upcoming is scheduled again.
func TestRuleRetriesFailedAiring(t *testing.T) {
	s, d, _ := newTestService(t)
	ctx := context.Background()
	rule, _, err := s.CreateRule(ctx, firstProgram(t, d), "missing", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	recs, _ := d.Recordings(ctx)
	failed := recs[0]
	if err := d.FinishRecording(ctx, failed.ID, "failed", "", 0, "all tuners were busy"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.reevaluate(ctx, rule.ID); err != nil {
		t.Fatal(err)
	}
	if r, _ := d.Recording(ctx, failed.ID); r.Status != "scheduled" {
		t.Fatalf("failed upcoming airing is %q after re-evaluation, want scheduled", r.Status)
	}
}

// A cancel that lands while a recording is starting wins.
func TestMarkRecordingRespectsCancel(t *testing.T) {
	_, d, _ := newTestService(t)
	ctx := context.Background()
	at := time.Now().Add(time.Hour).Unix()
	id, _, err := d.ScheduleRecording(ctx, db.Recording{Channel: "2.1", Title: "X", StartAt: at, EndAt: at + 1800}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.CancelRecording(ctx, id); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.MarkRecording(ctx, id, "/rec/x.ts"); err != nil || ok {
		t.Fatalf("MarkRecording after cancel = %v, %v; want false", ok, err)
	}
	if r, _ := d.Recording(ctx, id); r.Status != "cancelled" {
		t.Fatalf("status %q, want cancelled", r.Status)
	}
}

func TestRecordingNameEdges(t *testing.T) {
	n := recordingName(db.Recording{Title: "Show", EpisodeNum: "S00E05", StartAt: 1}, 0)
	if n.season != "Season 0" {
		t.Errorf("specials season folder = %q", n.season)
	}
	long := recordingName(db.Recording{Title: "Shōgun", EpisodeNum: "S01E01", EpisodeTitle: strings.Repeat("é", 200), StartAt: 1}, 0)
	if len(long.file) > 180 || !utf8.ValidString(long.file) {
		t.Errorf("long name: %d bytes, valid UTF-8 %v", len(long.file), utf8.ValidString(long.file))
	}
}

func TestKeepLastOrdersByEpisode(t *testing.T) {
	recs := []db.Recording{ // newest recorded first: a rerun of S01E02 was recorded last
		{ID: 1, EpisodeNum: "S01E02"}, {ID: 2, EpisodeNum: "S02E01"}, {ID: 3, EpisodeNum: "S01E09"},
	}
	newestEpisodesFirst(recs)
	if recs[0].ID != 2 || recs[1].ID != 3 || recs[2].ID != 1 {
		t.Errorf("order = %d %d %d, want 2 3 1", recs[0].ID, recs[1].ID, recs[2].ID)
	}
	mixed := []db.Recording{{ID: 1, EpisodeNum: ""}, {ID: 2, EpisodeNum: "S01E01"}}
	newestEpisodesFirst(mixed)
	if mixed[0].ID != 1 {
		t.Error("without episode numbers on all, recording order must stay")
	}
}
