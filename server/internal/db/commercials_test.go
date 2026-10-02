package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCommercialCandidates(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()

	lib, _ := d.CreateLibrary(ctx, "TV", "/media/TV", "tv")
	show, _, _ := d.EnsureItem(ctx, lib, "series", "Show", 0)
	add := func(ep int, problem string) int64 {
		e, _ := d.EnsureEpisode(ctx, show, 1, ep, "", "")
		id, err := d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: show, EpisodeID: &e, Problem: problem,
			Path: filepath.Join("/media/TV/Show", string(rune('a'+ep))+".ts"), Size: 10, Mtime: 10}, 1)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	e2, e1, e3 := add(2, ""), add(1, ""), add(3, "")
	add(4, "unreadable")
	if err := d.SetCommercials(ctx, e1, 10, 10, []Segment{{Start: 1, End: 2}}); err != nil {
		t.Fatal(err)
	}

	ids := func(redo bool) []int64 {
		fs, err := d.CommercialCandidates(ctx, show, redo)
		if err != nil {
			t.Fatal(err)
		}
		out := []int64{}
		for _, f := range fs {
			out = append(out, f.ID)
		}
		return out
	}
	// Unchecked, readable episodes in episode order; with redo, every readable one.
	if got := ids(false); len(got) != 2 || got[0] != e2 || got[1] != e3 {
		t.Errorf("unchecked = %v, want [%d %d]", got, e2, e3)
	}
	if got := ids(true); len(got) != 3 || got[0] != e1 || got[1] != e2 || got[2] != e3 {
		t.Errorf("redo = %v, want [%d %d %d]", got, e1, e2, e3)
	}
}
