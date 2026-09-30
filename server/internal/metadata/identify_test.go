package metadata

import (
	"context"
	"testing"
	"time"
)

type fakeSeries struct {
	matches []SeriesMatch
	seasons map[string][]Episode // imdb id → season 2 episodes
	calls   int
}

func (f *fakeSeries) SearchSeries(context.Context, string) ([]SeriesMatch, error) {
	return f.matches, nil
}
func (f *fakeSeries) Season(_ context.Context, id string, _ int) ([]Episode, error) {
	f.calls++
	return f.seasons[id], nil
}

func day(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func TestIdentifySeries(t *testing.T) {
	macgyver := func() *fakeSeries {
		return &fakeSeries{
			matches: []SeriesMatch{
				{ImdbID: "tt0088559", Title: "MacGyver", StartYear: 1985, EndYear: 1992},
				{ImdbID: "tt4786824", Title: "MacGyver", StartYear: 2016},
				{ImdbID: "tt9999999", Title: "MacGyver: Lost Treasure of Atlantis", StartYear: 1994, EndYear: 1994},
			},
			seasons: map[string][]Episode{
				"tt0088559": {{Season: 2, Episode: 5, Title: "Twice Stung", Released: "1986-10-20"}},
				"tt4786824": {{Season: 2, Episode: 5, Title: "Skull + Electrical Tape", Released: "2017-10-27"}},
			},
		}
	}
	cases := []struct {
		name string
		h    EpisodeHint
		want string
	}{
		{"air date in the new run", EpisodeHint{Title: "MacGyver", Season: 2, Episode: 5, AirDate: day("2017-10-27")}, "tt4786824"},
		{"rerun of the original", EpisodeHint{Title: "MacGyver", Season: 2, Episode: 5, AirDate: day("1986-10-20")}, "tt0088559"},
		{"no air date: episode title decides", EpisodeHint{Title: "MacGyver", Season: 2, Episode: 5, EpisodeTitle: "Twice Stung"}, "tt0088559"},
		{"no air date or episode info", EpisodeHint{Title: "MacGyver"}, ""},
		{"no air date, episode matches neither", EpisodeHint{Title: "MacGyver", Season: 2, Episode: 5, EpisodeTitle: "Something Else"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := IdentifySeries(context.Background(), macgyver(), c.h)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if m != nil {
				got = m.ImdbID
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}

	// One exact title is taken without any season lookups.
	f := &fakeSeries{matches: []SeriesMatch{{ImdbID: "tt0096697", Title: "The Simpsons", StartYear: 1989}, {ImdbID: "tt1", Title: "The Simpsons Shorts"}}}
	if m, _ := IdentifySeries(context.Background(), f, EpisodeHint{Title: "The Simpsons", Season: 30, Episode: 1}); m == nil || m.ImdbID != "tt0096697" || f.calls != 0 {
		t.Fatalf("single match = %+v, calls %d", m, f.calls)
	}
}

func TestYearRange(t *testing.T) {
	for in, want := range map[string][2]int{"1985–1992": {1985, 1992}, "2016–": {2016, 0}, "2010": {2010, 2010}, "2003-2011": {2003, 2011}, "": {0, 0}} {
		if s, e := yearRange(in); s != want[0] || e != want[1] {
			t.Errorf("yearRange(%q) = %d, %d; want %v", in, s, e, want)
		}
	}
}
