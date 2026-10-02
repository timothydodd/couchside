package db

import (
	"context"
	"testing"
)

func TestCreditsAndPersonItems(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	lib, _ := d.CreateLibrary(bg, "Movies", "/m", "movies")
	a, _, _ := d.EnsureItem(bg, lib, "movie", "The Matrix", 1999)
	b, _, _ := d.EnsureItem(bg, lib, "movie", "John Wick", 2014)
	if err := d.SetCredits(bg, a, []Credit{
		{PersonID: 6384, Name: "Keanu Reeves", ProfilePath: "/k.jpg", Kind: "cast", Role: "Neo", Order: 0},
		{PersonID: 2975, Name: "Laurence Fishburne", Kind: "cast", Role: "Morpheus", Order: 1},
		{PersonID: 9339, Name: "Lana Wachowski", Kind: "crew", Role: "Director", Order: 100},
		{PersonID: 9339, Name: "Lana Wachowski", Kind: "crew", Role: "Screenplay", Order: 101},
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetCredits(bg, b, []Credit{{PersonID: 6384, Name: "Keanu Reeves", Kind: "cast", Role: "John Wick"}}); err != nil {
		t.Fatal(err)
	}
	cast, crew, err := d.ItemCredits(bg, a)
	if err != nil || len(cast) != 2 || cast[0].Name != "Keanu Reeves" || !cast[0].HasPhoto || cast[1].HasPhoto {
		t.Fatalf("cast = %+v, %v", cast, err)
	}
	if len(crew) != 1 || crew[0].Role != "Director, Screenplay" {
		t.Fatalf("crew = %+v (a writer-director is one row)", crew)
	}
	items, err := d.PersonItems(bg, 6384)
	if err != nil || len(items) != 2 || items[0].ID != b || items[0].Roles[0] != "John Wick" || items[1].Roles[0] != "Neo" {
		t.Fatalf("person items = %+v, %v", items, err)
	}
	// Re-matching replaces credits; clearing a match drops them.
	if err := d.SetCredits(bg, a, nil); err != nil {
		t.Fatal(err)
	}
	if items, _ := d.PersonItems(bg, 6384); len(items) != 1 {
		t.Fatalf("after replacing credits: %+v", items)
	}
	_ = d.SetCredits(bg, b, []Credit{{PersonID: 6384, Name: "Keanu Reeves", Kind: "cast"}})
	if err := d.ClearMatch(bg, b); err != nil {
		t.Fatal(err)
	}
	if items, _ := d.PersonItems(bg, 6384); len(items) != 0 {
		t.Fatalf("after clearing the match: %+v", items)
	}
}
