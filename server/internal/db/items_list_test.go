package db

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

// oldSummaryCols is summaryCols as Items used it before the aggregated join,
// kept here so the join is checked against it, not against itself.
func oldItems(ctx context.Context, d *DB, kind string) ([]ItemSummary, error) {
	return d.querySummaries(ctx, `SELECT `+summaryCols(ctx)+` FROM media_items m WHERE m.kind = ?`+visible(ctx, "m")+` ORDER BY m.sort_title`, kind)
}

// seedLibrary makes n movies with a mix of copies, extras, dynamic ranges,
// watched files and My list entries for profile me.
func seedLibrary(t testing.TB, d *DB, me int64, n int) {
	t.Helper()
	ctx := context.Background()
	mine := WithProfile(ctx, me)
	lib, err := d.CreateLibrary(ctx, "Films", "/films", "movies")
	if err != nil {
		t.Fatal(err)
	}
	ranges := []string{"", "hdr10", "dv", "hlg"}
	for i := 0; i < n; i++ {
		item, _, err := d.EnsureItem(ctx, lib, "movie", fmt.Sprintf("Film %05d", i), 1990+i%30)
		if err != nil {
			t.Fatal(err)
		}
		files := 1 + i%3 // one to three files
		var ids []int64
		for j := 0; j < files; j++ {
			id, err := d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: item, Path: fmt.Sprintf("/films/%d/%d.mkv", i, j), Size: int64(j + 1), Mtime: 1}, 1)
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
			if err := d.SetDynamicRange(ctx, id, ranges[(i+j)%4], 0); err != nil {
				t.Fatal(err)
			}
		}
		if files > 1 && i%2 == 0 { // the last copy is an extra, in DV, which mustn't count
			if err := d.SetFileRole(ctx, ids[files-1], "extra", 0, "Trailer"); err != nil {
				t.Fatal(err)
			}
			_ = d.SetDynamicRange(ctx, ids[files-1], "dv", 5)
		}
		if i%5 == 0 {
			if err := d.SetWatched(mine, ids[:1], true); err != nil {
				t.Fatal(err)
			}
		}
		if i%7 == 0 {
			if err := d.SetWatchlist(mine, item, true); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Items' one-pass join answers exactly what the per-title subqueries did,
// for the profile with the watched files and list, another profile, and
// background work (no profile).
func TestItemsJoinMatchesSubqueries(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	me, err := d.SetupAdmin(ctx, "Me", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other, err := d.CreateAccount(ctx, "Other", "accent", "user", false, "h", false)
	if err != nil {
		t.Fatal(err)
	}
	seedLibrary(t, d, me.ID, 60)
	for _, c := range []context.Context{WithProfile(ctx, me.ID), WithProfile(ctx, other.ID), ctx} {
		got, err := d.Items(c, "movie")
		if err != nil {
			t.Fatal(err)
		}
		want, err := oldItems(c, d, "movie")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 60 || !reflect.DeepEqual(got, want) {
			for i := range want {
				if i < len(got) && !reflect.DeepEqual(got[i], want[i]) {
					t.Fatalf("profile %d, row %d:\n got %+v\nwant %+v", ProfileID(c), i, got[i], want[i])
				}
			}
			t.Fatalf("profile %d: %d rows, want %d", ProfileID(c), len(got), len(want))
		}
	}
}

func BenchmarkItems5k(b *testing.B) {
	d, err := Open(b.TempDir() + "/b.db")
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()
	me, _ := d.SetupAdmin(context.Background(), "Me", "hash")
	seedLibrary(b, d, me.ID, 5000)
	ctx := WithProfile(context.Background(), me.ID)
	b.Run("join", func(b *testing.B) {
		for range b.N {
			if _, err := d.Items(ctx, "movie"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("subqueries", func(b *testing.B) {
		for range b.N {
			if _, err := oldItems(ctx, d, "movie"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
