package db

import (
	"context"
	"path/filepath"
	"testing"
)

// A title is one item however many libraries hold files for it: the second
// library's scan finds the first's item; pruning one library's files leaves
// the item, re-homed to the other; deleting a library keeps shared titles.
func TestItemsAreSharedAcrossLibraries(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	tv, _ := d.CreateLibrary(ctx, "TV", filepath.Join(dir, "tv"), "tv")
	dvr, _ := d.CreateLibrary(ctx, "DVR", filepath.Join(dir, "dvr"), "tv")

	a, created, err := d.EnsureItem(ctx, tv, "series", "The Goldbergs", 2013)
	if err != nil || !created {
		t.Fatalf("first: %d %v %v", a, created, err)
	}
	b, created, err := d.EnsureItem(ctx, dvr, "series", "The Goldbergs", 2013)
	if err != nil || created || b != a {
		t.Fatalf("second library: item %d (created %v), want %d again", b, created, a)
	}
	if other, _, _ := d.EnsureItem(ctx, dvr, "series", "The Goldbergs", 0); other == a {
		t.Fatal("a different parsed year is a different title")
	}
	ep1, _ := d.EnsureEpisode(ctx, a, 1, 1, "Pilot", "")
	ep2, _ := d.EnsureEpisode(ctx, a, 11, 3, "", "")
	f1, _ := d.UpsertFile(ctx, File{LibraryID: tv, MediaItemID: a, EpisodeID: &ep1, Path: filepath.Join(dir, "tv", "g1.mkv"), Size: 1, Mtime: 1}, 1)
	f2, _ := d.UpsertFile(ctx, File{LibraryID: dvr, MediaItemID: a, EpisodeID: &ep2, Path: filepath.Join(dir, "dvr", "g2.ts"), Size: 1, Mtime: 1}, 1)

	libs, _ := d.Libraries(ctx)
	for _, l := range libs {
		if l.ID != tv && l.ID != dvr {
			continue
		}
		if l.ItemCount != 1 || l.FileCount != 1 {
			t.Errorf("library %s counts items=%d files=%d, want 1 and 1", l.Name, l.ItemCount, l.FileCount)
		}
	}
	rows, _ := d.ManageRows(ctx, dvr)
	if len(rows) != 1 || rows[0].ID != a || rows[0].FileCount != 2 {
		t.Fatalf("DVR manage rows = %+v, want the shared show with both files", rows)
	}

	// Only the TV library is limited: the title is visible through its DVR file too.
	limited := WithAccess(ctx, Access{Libraries: []int64{dvr}})
	if items, _ := d.Items(limited, "series"); len(items) != 1 {
		t.Fatalf("a profile limited to DVR sees %d series, want 1", len(items))
	}
	if files, _ := d.ItemFiles(limited, a); len(files) != 1 || files[0].ID != f2 {
		t.Fatalf("that profile's files = %+v, want only the DVR one", files)
	}

	// The TV library's file goes away: the show stays, now homed in DVR.
	if _, err := d.PruneLibrary(ctx, tv, 2); err != nil { // f1 was last seen at 1
		t.Fatal(err)
	}
	it, err := d.Item(ctx, a)
	if err != nil || it.LibraryID != dvr {
		t.Fatalf("after pruning TV: item %+v, %v; want it homed in DVR", it, err)
	}
	if _, err := d.File(ctx, f1); err == nil {
		t.Fatal("the pruned file is still there")
	}
	// Deleting the DVR library takes the last file and the show with it.
	if err := d.DeleteLibrary(ctx, dvr); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Item(ctx, a); err == nil {
		t.Fatal("a show with no files left survived its library")
	}
}
