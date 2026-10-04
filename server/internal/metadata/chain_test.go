package metadata

import (
	"context"
	"errors"
	"testing"
)

type stubProvider struct {
	name string
	d    *Details
	err  error
}

func (s stubProvider) Name() string { return s.name }
func (s stubProvider) Lookup(context.Context, Kind, string, int) (*Details, error) {
	return s.d, s.err
}
func (s stubProvider) ByImdbID(context.Context, Kind, string) (*Details, error) { return s.d, s.err }
func (s stubProvider) Season(context.Context, string, int) ([]Episode, error)   { return nil, nil }

func TestChainReportsASkippedProvider(t *testing.T) {
	ctx := context.Background()
	down := stubProvider{name: "tmdb", err: errors.New("tmdb: rate limited")}
	none := stubProvider{name: "tmdb"}
	full := stubProvider{name: "tmdb", d: &Details{Title: "Heat", Credits: []Credit{{PersonID: 1, Name: "Al Pacino"}}}}
	omdb := stubProvider{name: "omdb", d: &Details{Title: "Heat"}}
	omdbNone := stubProvider{name: "omdb"}

	// The first provider is down and the second answers: a match, flagged.
	d, p, skipped, err := (&Chain{Providers: []Provider{down, omdb}}).Lookup(ctx, Movie, "Heat", 1995, "")
	if err != nil || d == nil || p.Name() != "omdb" || skipped == nil {
		t.Fatalf("down then match: d=%v p=%v skipped=%v err=%v", d, p, skipped, err)
	}
	// The first provider answers: nothing was skipped.
	if d, _, skipped, err := (&Chain{Providers: []Provider{full, omdb}}).Lookup(ctx, Movie, "Heat", 1995, ""); err != nil || d == nil || skipped != nil {
		t.Fatalf("first answers: d=%v skipped=%v err=%v", d, skipped, err)
	}
	// The first has no match (not an error) and the second does: not skipped.
	if d, _, skipped, err := (&Chain{Providers: []Provider{none, omdb}}).Lookup(ctx, Movie, "Heat", 1995, ""); err != nil || d == nil || skipped != nil {
		t.Fatalf("no match then match: d=%v skipped=%v err=%v", d, skipped, err)
	}
	// Down, and nobody else matches: an error, so the caller tries again later.
	if d, _, _, err := (&Chain{Providers: []Provider{down, omdbNone}}).Lookup(ctx, Movie, "Heat", 1995, ""); err == nil || d != nil {
		t.Fatalf("down and no match: d=%v err=%v", d, err)
	}
	// By IMDb id, the same.
	if _, _, skipped, err := (&Chain{Providers: []Provider{down, omdb}}).Lookup(ctx, Movie, "", 0, "tt0113277"); err != nil || skipped == nil {
		t.Fatalf("by id: skipped=%v err=%v", skipped, err)
	}
}
