package livetv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/metadata"
)

// fakeOMDb knows both MacGyvers.
type fakeOMDb struct{ searches int }

func (f *fakeOMDb) Name() string { return "fake" }
func (f *fakeOMDb) Lookup(context.Context, metadata.Kind, string, int) (*metadata.Details, error) {
	return nil, nil
}
func (f *fakeOMDb) ByImdbID(context.Context, metadata.Kind, string) (*metadata.Details, error) {
	return nil, nil
}
func (f *fakeOMDb) Season(context.Context, string, int) ([]metadata.Episode, error) { return nil, nil }
func (f *fakeOMDb) SearchSeries(_ context.Context, title string) ([]metadata.SeriesMatch, error) {
	f.searches++
	if title != "MacGyver" {
		return []metadata.SeriesMatch{{ImdbID: "tt0096697", Title: title, StartYear: 1989}}, nil
	}
	return []metadata.SeriesMatch{
		{ImdbID: "tt0088559", Title: "MacGyver", StartYear: 1985, EndYear: 1992},
		{ImdbID: "tt4786824", Title: "MacGyver", StartYear: 2016, EndYear: 2021},
	}, nil
}

func TestShowYearTellsSameTitledSeriesApart(t *testing.T) {
	s, d, _ := newTestService(t)
	fake := &fakeOMDb{}
	s.cfg.Metadata = &metadata.Chain{Providers: []metadata.Provider{fake}}
	ctx := context.Background()

	at := time.Now().Add(3 * time.Hour).Truncate(time.Hour).Unix()
	aired := time.Date(2017, 10, 27, 0, 0, 0, 0, time.UTC).Unix()
	if err := d.UpsertPrograms(ctx, []db.Program{
		{Channel: "2.1", StartAt: at, EndAt: at + 3600, Title: "MacGyver", EpisodeNum: "S02E05", SeriesID: "SH-NEW", OriginalAirdate: &aired},
		{Channel: "5.1", StartAt: at, EndAt: at + 1800, Title: "The Simpsons", EpisodeNum: "S07E05", SeriesID: "SH-SIMP"},
	}); err != nil {
		t.Fatal(err)
	}
	mac := db.Recording{Channel: "2.1", StartAt: at, Title: "MacGyver", EpisodeNum: "S02E05", EpisodeTitle: "Skull", SeriesID: "SH-NEW"}
	if y := s.showYear(ctx, mac, true); y != 2016 {
		t.Fatalf("MacGyver year = %d, want 2016", y)
	}
	// Stored per guide series: no second lookup.
	if y := s.showYear(ctx, mac, true); y != 2016 || fake.searches != 1 {
		t.Fatalf("second call: year %d, searches %d", y, fake.searches)
	}
	// A title only one series has keeps its plain folder name.
	simp := db.Recording{Channel: "5.1", StartAt: at, Title: "The Simpsons", EpisodeNum: "S07E05", SeriesID: "SH-SIMP"}
	if y := s.showYear(ctx, simp, true); y != 0 {
		t.Fatalf("Simpsons year = %d, want 0", y)
	}
	if m, _ := d.SeriesMatchFor(ctx, "SH-SIMP"); m == nil || m.ImdbID != "tt0096697" {
		t.Fatalf("Simpsons stored match = %+v", m)
	}
}

func TestRecordingAvoidsOtherSeriesFolder(t *testing.T) {
	s, d, _ := newTestService(t)
	ctx := context.Background()
	dir := t.TempDir()
	// An existing plain "MacGyver" folder holding the 1985 series.
	old := filepath.Join(dir, "MacGyver", "Season 02")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	lib, _ := d.CreateLibrary(ctx, "Shows", dir, "tv")
	item, _, _ := d.EnsureItem(ctx, lib, "series", "MacGyver", 0)
	if err := d.ApplyMetadata(ctx, item, db.Metadata{Title: "MacGyver", Year: 1985, ImdbID: "tt0088559"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: filepath.Join(old, "MacGyver - S02E05.mkv"), Size: 1, Mtime: 1}, 1); err != nil {
		t.Fatal(err)
	}
	rec := db.Recording{Title: "MacGyver", EpisodeNum: "S02E05", EpisodeTitle: "Skull", StartAt: time.Now().Unix()}

	got := s.pathInDir(dir, recordingName(rec, 2016))
	want := filepath.Join(dir, "MacGyver (2016)", "Season 2", "MacGyver (2016) - S02E05 - Skull.ts")
	if got != want {
		t.Errorf("2016 recording:\n got  %s\n want %s", got, want)
	}
	// The 1985 series (or an unknown one) still joins the existing folder.
	for _, y := range []int{1985, 0} {
		if got := s.pathInDir(dir, recordingName(rec, y)); filepath.Dir(got) != old {
			t.Errorf("year %d: got %s, want it in %s", y, got, old)
		}
	}
	// Once "MacGyver (2016)" exists, the 2016 series goes there.
	if err := os.MkdirAll(filepath.Join(dir, "MacGyver (2016)", "Season 02"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := s.pathInDir(dir, recordingName(rec, 2016)); filepath.Dir(got) != filepath.Join(dir, "MacGyver (2016)", "Season 02") {
		t.Errorf("existing 2016 folder: got %s", got)
	}
}
