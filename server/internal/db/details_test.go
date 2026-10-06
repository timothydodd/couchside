package db

import (
	"context"
	"path/filepath"
	"testing"
)

// Details set by hand win over the provider's, match after match, and a
// cleared one goes back to the provider's on the next match.
func TestItemDetailsOverrideTheProvider(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	lib, _ := d.CreateLibrary(ctx, "Films", dir, "movies")
	id, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat", 1995)
	provider := Metadata{Title: "Heat", Year: 1995, Plot: "A crew of thieves.", Genres: []string{"Crime", "Drama"}, Rated: "R", Provider: "tmdb"}
	if err := d.ApplyMetadata(ctx, id, provider); err != nil {
		t.Fatal(err)
	}
	title, plot := "Heat (Director's Cut)", "My own words."
	if cleared, err := d.SetItemDetails(ctx, id, Overrides{Title: &title, Plot: &plot}); err != nil || cleared {
		t.Fatalf("set: cleared=%v err=%v", cleared, err)
	}
	it, _ := d.Item(ctx, id)
	if it.Title != title || it.Plot != plot || it.Rated != "R" || it.SortTitle != SortTitle(title) {
		t.Fatalf("after set: %+v", it)
	}
	if it.Overrides.Title == nil || *it.Overrides.Title != title || it.Overrides.Rated != nil {
		t.Fatalf("overrides = %+v", it.Overrides)
	}
	// The provider answers again (a re-match): the hand-set fields stay.
	provider.Plot = "A crew of thieves, again."
	provider.Rated = "PG-13"
	if err := d.ApplyMetadata(ctx, id, provider); err != nil {
		t.Fatal(err)
	}
	it, _ = d.Item(ctx, id)
	if it.Title != title || it.Plot != plot || it.Rated != "PG-13" {
		t.Fatalf("after re-match: %+v", it)
	}
	// Clearing the title: reported, so the caller fetches the provider's again.
	if cleared, err := d.SetItemDetails(ctx, id, Overrides{Plot: &plot}); err != nil || !cleared {
		t.Fatalf("clear: cleared=%v err=%v", cleared, err)
	}
	if err := d.ApplyMetadata(ctx, id, provider); err != nil {
		t.Fatal(err)
	}
	it, _ = d.Item(ctx, id)
	if it.Title != "Heat" || it.Plot != plot {
		t.Fatalf("after clearing the title and re-matching: %+v", it)
	}
	// Even an unmatch keeps what was set by hand.
	if err := d.ClearMatch(ctx, id); err != nil {
		t.Fatal(err)
	}
	if it, _ = d.Item(ctx, id); it.Plot != plot || it.MatchStatus != "unmatched" {
		t.Fatalf("after ClearMatch: %+v", it)
	}
}
