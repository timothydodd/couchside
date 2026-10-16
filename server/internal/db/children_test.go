package db

import (
	"context"
	"path/filepath"
	"testing"
)

// A parent with no file of its own lives while titles are filed under it,
// and goes with the last of them.
func TestTidyKeepsParentWithChildren(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := WithProfile(context.Background(), 1)
	lib, _ := d.CreateLibrary(ctx, "Films", dir, "movies")
	set, _, _ := d.EnsureItem(ctx, lib, "movie", "Looney Tunes Platinum Collection", 2011)
	door, _, _ := d.EnsureItem(ctx, lib, "movie", "The Door", 1968)
	f, _ := d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: door, Path: filepath.Join(dir, "set", "d3", "door.mp4"), Size: 1, Mtime: 1}, 1)
	under := filepath.Join(dir, "set") + string(filepath.Separator)
	if err := d.SetDetectedParent(ctx, door, &set, "Bonus Cartoons", 5, under); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PruneLibrary(ctx, lib, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Item(ctx, set); err != nil {
		t.Fatalf("the set was tidied away while it had a child: %v", err)
	}
	// The set is in the grid (with no files), the child isn't; the child lists under it.
	top, _ := d.Items(ctx, "movie", false)
	if len(top) != 1 || top[0].ID != set || top[0].ChildCount != 1 {
		t.Fatalf("grid = %+v", top)
	}
	all, _ := d.Items(ctx, "movie", true)
	if len(all) != 2 {
		t.Fatalf("with children = %d, want 2", len(all))
	}
	kids, _ := d.ChildItems(ctx, set)
	if len(kids) != 1 || kids[0].FileID != f || kids[0].GroupLabel != "Bonus Cartoons" || kids[0].ParentTitle != "Looney Tunes Platinum Collection" {
		t.Fatalf("children = %+v", kids)
	}
	// A profile limited to the library sees the set through its child.
	limited := WithAccess(ctx, Access{Libraries: []int64{lib}})
	if top, _ := d.Items(limited, "movie", false); len(top) != 1 {
		t.Fatalf("limited profile's grid = %+v, want the set", top)
	}
	// A detected parent doesn't claim a title that also has a file elsewhere.
	heat, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat", 1995)
	_, _ = d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: heat, Path: filepath.Join(dir, "Heat (1995).mkv"), Size: 1, Mtime: 1}, 1)
	_, _ = d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: heat, Path: filepath.Join(dir, "set", "d1", "heat.mkv"), Size: 2, Mtime: 1}, 1)
	if err := d.SetDetectedParent(ctx, heat, &set, "", 0, under); err != nil {
		t.Fatal(err)
	}
	if it, _ := d.Item(ctx, heat); it.ParentID != nil {
		t.Fatalf("Heat, with a file outside the set, was filed under it")
	}
	// Unfiled, the set has nothing and goes on the next tidy.
	if err := d.SetDetectedParent(ctx, door, nil, "", 0, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PruneLibrary(ctx, lib, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Item(ctx, set); err == nil {
		t.Fatal("the empty set is still there")
	}
}

func TestSetParentRules(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := WithProfile(context.Background(), 1)
	lib, _ := d.CreateLibrary(ctx, "Films", dir, "movies")
	tv, _ := d.CreateLibrary(ctx, "Shows", filepath.Join(dir, "tv"), "tv")
	a, _, _ := d.EnsureItem(ctx, lib, "movie", "A", 0)
	b, _, _ := d.EnsureItem(ctx, lib, "movie", "B", 0)
	c, _, _ := d.EnsureItem(ctx, lib, "movie", "C", 0)
	show, _, _ := d.EnsureItem(ctx, tv, "series", "Show", 0)
	if err := d.SetParent(ctx, b, &a); err != nil {
		t.Fatal(err)
	}
	if err := d.SetParent(ctx, a, &a); err == nil {
		t.Error("filed under itself")
	}
	if err := d.SetParent(ctx, c, &b); err == nil {
		t.Error("filed under a child")
	}
	if err := d.SetParent(ctx, a, &c); err == nil {
		t.Error("a parent was filed under another")
	}
	if err := d.SetParent(ctx, show, &a); err == nil {
		t.Error("a show was filed")
	}
	if err := d.SetParent(ctx, c, &show); err == nil {
		t.Error("filed under a show")
	}
	// Hand-filed sticks: the scan's guess doesn't move it.
	if err := d.SetDetectedParent(ctx, b, nil, "", 0, ""); err != nil {
		t.Fatal(err)
	}
	if it, _ := d.Item(ctx, b); it.ParentID == nil || *it.ParentID != a || it.ParentTitle != "A" {
		t.Fatalf("b = %+v, want filed under A", it.ItemSummary)
	}
	if err := d.SetParent(ctx, b, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.SetDetectedParent(ctx, b, &a, "", 0, ""); err != nil {
		t.Fatal(err)
	}
	if it, _ := d.Item(ctx, b); it.ParentID != nil {
		t.Fatal("unfiled by hand, but the scan filed it again")
	}
	// An empty title can be deleted; one with a file can't this way.
	if err := d.DeleteEmptyItem(ctx, a); err != nil {
		t.Fatal(err)
	}
	_, _ = d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: c, Path: filepath.Join(dir, "c.mkv"), Size: 1, Mtime: 1}, 1)
	if err := d.DeleteEmptyItem(ctx, c); err == nil {
		t.Fatal("deleted a title that has a file")
	}
}
