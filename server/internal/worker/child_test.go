package worker

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/db"
)

// A box set folder with bonus discs and no film of its own: the set becomes
// a title, every nested file a child filed under it.
func TestScanNestedChildren(t *testing.T) {
	w, d, lib, root := scanFixture(t)
	w.cfg.NestedChildren = true
	ctx := db.WithProfile(context.Background(), 1)
	set := filepath.Join(root, "Looney Tunes Platinum Collection - Volume 1 (2011)")
	greeting := filepath.Join(set, "Disc 1 - Behind the Tunes", "01 - A Greeting from Chuck Jones.mp4")
	door := filepath.Join(set, "Disc 3 - Bonus Materials", "Bonus Cartoons", "05 - The Door (1968).mp4")
	trailer := filepath.Join(set, "Disc 3 - Bonus Materials", "Trailers", "Trailer.mp4")
	heat := filepath.Join(root, "Heat (1995).mkv")
	for _, p := range []string{greeting, door, trailer, heat} {
		writeVideo(t, p)
	}
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	top, err := d.Items(ctx, "movie", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 {
		t.Fatalf("Movies grid = %v, want the set and Heat", titles(top))
	}
	var set2 db.ItemSummary
	for _, it := range top {
		if it.Title == "Looney Tunes Platinum Collection - Volume 1" {
			set2 = it
		}
	}
	if set2.ID == 0 || set2.ChildCount != 2 || set2.FileCount != 0 {
		t.Fatalf("the set = %+v, want two children and no file of its own", set2)
	}
	children, err := d.ChildItems(ctx, set2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 || children[0].Title != "A Greeting from Chuck Jones" || children[0].GroupLabel != "Behind the Tunes" || children[0].SortNo != 1 ||
		children[1].Title != "The Door" || children[1].Year == nil || *children[1].Year != 1968 || children[1].GroupLabel != "Bonus Materials · Bonus Cartoons" || children[1].FileID == 0 {
		t.Fatalf("children = %+v", children)
	}
	// The Trailers folder on the disc is extras of the set itself.
	files, _ := d.ItemFiles(ctx, set2.ID)
	if len(files) != 1 || files[0].Role != "extra" || files[0].ExtraTitle != "Trailer" {
		t.Fatalf("set's files = %+v, want one extra", files)
	}
	// The set is matched like any new title.
	if jobs, _ := d.RecentJobs(ctx, 50); !hasJob(jobs, KindMatch, set2.ID) {
		t.Fatalf("no match queued for the set: %+v", jobs)
	}
	// All is well on a second scan: nothing re-filed, the set stays.
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if again, _ := d.Items(ctx, "movie", false); len(again) != 2 {
		t.Fatalf("after rescan: %v", titles(again))
	}

	// With the rule off, the next scan takes the children out and the set goes.
	w.cfg.NestedChildren = false
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	flat, _ := d.Items(ctx, "movie", false)
	if len(flat) != 4 {
		t.Fatalf("nesting off: Movies grid = %v, want four films (the trailer stays the set's extra)", titles(flat))
	}
	for _, it := range flat {
		if it.ParentID != nil {
			t.Errorf("%s still filed under %d", it.Title, *it.ParentID)
		}
	}
	// And back on: filed again, under the same set.
	w.cfg.NestedChildren = true
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if again, _ := d.Items(ctx, "movie", false); len(again) != 2 {
		t.Fatalf("nesting back on: %v", titles(again))
	}
}

// A child filed by hand elsewhere, or unfiled by hand, is left where the admin put it.
func TestScanKeepsHandFiledParent(t *testing.T) {
	w, d, lib, root := scanFixture(t)
	w.cfg.NestedChildren = true
	ctx := db.WithProfile(context.Background(), 1)
	door := filepath.Join(root, "Looney Tunes (2011)", "Bonus", "Cartoons", "The Door (1968).mp4")
	writeVideo(t, door)
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	st, _ := d.FileStamp(ctx, door)
	if st == nil || st.ParentID == nil {
		t.Fatalf("not filed: %+v", st)
	}
	if err := d.SetParent(ctx, st.MediaItemID, nil); err != nil {
		t.Fatal(err)
	}
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if st, _ = d.FileStamp(ctx, door); st.ParentID != nil {
		t.Fatalf("the scan filed an unfiled title again: %+v", st)
	}
}

func titles(items []db.ItemSummary) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}

func hasJob(jobs []db.Job, kind string, ref int64) bool {
	for _, j := range jobs {
		if j.Kind == kind && j.RefID == ref {
			return true
		}
	}
	return false
}
