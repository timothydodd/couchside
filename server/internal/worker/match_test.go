package worker

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/metadata"
)

type stubProvider struct {
	name string
	d    *metadata.Details
	err  error
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Lookup(context.Context, metadata.Kind, string, int) (*metadata.Details, error) {
	return s.d, s.err
}
func (s *stubProvider) ByImdbID(context.Context, metadata.Kind, string) (*metadata.Details, error) {
	return s.d, s.err
}
func (s *stubProvider) Season(context.Context, string, int) ([]metadata.Episode, error) {
	return nil, nil
}

// TMDB down while OMDb answers: the item keeps its cast and backdrop, and
// stays pending so the next scan matches it again. It used to be marked
// matched with both wiped.
func TestMatchKeepsCreditsWhenAProviderIsDown(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	lib, _ := d.CreateLibrary(ctx, "Films", "/films", "movies")
	item, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat", 1995)

	tmdb := &stubProvider{name: "tmdb", d: &metadata.Details{Title: "Heat", Year: 1995, ImdbID: "tt0113277",
		PosterURL: "https://image.tmdb.org/p.jpg", BackdropURL: "https://image.tmdb.org/b.jpg",
		Credits: []metadata.Credit{{PersonID: 1158, Name: "Al Pacino", Kind: "cast", Role: "Vincent Hanna"}}}}
	omdb := &stubProvider{name: "omdb", d: &metadata.Details{Title: "Heat", Year: 1995, ImdbID: "tt0113277", PosterURL: "https://m.media-amazon.com/p.jpg"}}
	w := &Worker{db: d, cfg: config.Config{}, providers: &metadata.Chain{Providers: []metadata.Provider{tmdb, omdb}},
		wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}

	state := func() (status, backdrop string, cast int) {
		t.Helper()
		it, err := d.Item(ctx, item)
		if err != nil {
			t.Fatal(err)
		}
		c, _, err := d.ItemCredits(ctx, item)
		if err != nil {
			t.Fatal(err)
		}
		return it.MatchStatus, it.BackdropURL, len(c)
	}

	if err := w.match(ctx, item); err != nil {
		t.Fatal(err)
	}
	if status, backdrop, cast := state(); status != "matched" || backdrop == "" || cast != 1 {
		t.Fatalf("first match: %s, backdrop %q, cast %d", status, backdrop, cast)
	}

	tmdb.err, tmdb.d = errors.New("tmdb: rate limited (HTTP 429); will retry"), nil
	if err := w.match(ctx, item); err != nil {
		t.Fatal(err)
	}
	status, backdrop, cast := state()
	if backdrop == "" || cast != 1 {
		t.Fatalf("a match during a TMDB outage wiped the backdrop (%q) or cast (%d)", backdrop, cast)
	}
	if status != "pending" {
		t.Fatalf("status = %s, want pending so the next scan retries", status)
	}
	pending, err := d.PendingMatches(ctx, lib)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending matches = %v, %v", pending, err)
	}

	// TMDB is back and no longer lists anyone: that is an answer, so it applies.
	tmdb.err, tmdb.d = nil, &metadata.Details{Title: "Heat", Year: 1995, ImdbID: "tt0113277"}
	if err := w.match(ctx, item); err != nil {
		t.Fatal(err)
	}
	if status, backdrop, cast := state(); status != "matched" || backdrop != "" || cast != 0 {
		t.Fatalf("after recovery: %s, backdrop %q, cast %d", status, backdrop, cast)
	}
}
