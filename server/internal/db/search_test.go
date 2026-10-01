package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSearchRowsAndHydration(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	bg := context.Background()
	me := WithProfile(bg, 1)

	lib, err := d.CreateLibrary(bg, "TV", "/tv", "tv")
	if err != nil {
		t.Fatal(err)
	}
	show, _, err := d.EnsureItem(bg, lib, "series", "The Office", 2005)
	if err != nil {
		t.Fatal(err)
	}
	ep1, _ := d.EnsureEpisode(bg, show, 4, 13, "Dinner Party", "")
	if _, err := d.EnsureEpisode(bg, show, 4, 14, "Goodbye, Toby", ""); err != nil { // no file: not searchable
		t.Fatal(err)
	}
	small, err := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: show, EpisodeID: &ep1, Path: "/tv/a.mkv", Size: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	big, err := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: show, EpisodeID: &ep1, Path: "/tv/b.mkv", Size: 2}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SaveProgress(me, big, 60, 1300); err != nil {
		t.Fatal(err)
	}

	rows, err := d.SearchRows(bg, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, r := range rows {
		kinds[r.Kind]++
	}
	if kinds["series"] != 1 || kinds["episode"] != 1 || kinds["channel"] != 0 {
		t.Fatalf("rows = %+v", rows)
	}

	plays, err := d.EpisodePlays(me, []int64{ep1, 999})
	if err != nil || len(plays) != 1 {
		t.Fatalf("plays = %+v, %v", plays, err)
	}
	if p := plays[0]; p.FileID != big || p.PositionSec != 60 || p.Title != "The Office" || p.Subtitle != "S4 · E13 · Dinner Party" {
		t.Fatalf("play = %+v (small file %d)", p, small)
	}

	items, err := d.ItemsByID(me, []int64{999, show})
	if err != nil || len(items) != 1 || items[0].ID != show {
		t.Fatalf("items = %+v, %v", items, err)
	}
}
