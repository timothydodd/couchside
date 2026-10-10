package db

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
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

// TouchFiles marks exactly the files given, across its 500-id chunks, and
// FileStamps reads the same stamps FileStamp does, for one library only.
func TestTouchFilesAndStamps(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	a, _ := d.CreateLibrary(ctx, "A", "/a", "movies")
	b, _ := d.CreateLibrary(ctx, "B", "/b", "movies")
	item, _, _ := d.EnsureItem(ctx, a, "movie", "Heat", 1995)
	var ids []int64
	for i := 0; i < 1201; i++ {
		id, err := d.UpsertFile(ctx, File{LibraryID: a, MediaItemID: item, Path: fmt.Sprintf("/a/%d.mkv", i), Size: 1, Mtime: 1}, 1)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	other, _ := d.UpsertFile(ctx, File{LibraryID: b, MediaItemID: item, Path: "/b/x.mkv", Size: 1, Mtime: 1}, 1)
	if err := d.TouchFiles(ctx, ids, 99); err != nil {
		t.Fatal(err)
	}
	var touched, untouched int
	d.sql.QueryRow(`SELECT COUNT(*) FROM files WHERE last_seen = 99`).Scan(&touched)
	d.sql.QueryRow(`SELECT COUNT(*) FROM files WHERE id = ? AND last_seen = 1`, other).Scan(&untouched)
	if touched != 1201 || untouched != 1 {
		t.Fatalf("touched %d (want 1201), other library untouched %d", touched, untouched)
	}
	stamps, err := d.FileStamps(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	one, _ := d.FileStamp(ctx, "/a/7.mkv")
	if len(stamps) != 1201 || stamps["/b/x.mkv"] != nil || !reflect.DeepEqual(stamps["/a/7.mkv"], one) {
		t.Fatalf("stamps: %d, other library's %v, /a/7 %+v vs %+v", len(stamps), stamps["/b/x.mkv"], stamps["/a/7.mkv"], one)
	}
}

// Jobs come out in the same order as before claims went kind by kind: scan,
// match, artwork, then the rest oldest first; commercials, optimize,
// intros, trickplay in the encode pool. A kind added later isn't stranded.
func TestClaimJobOrder(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	for i, k := range []string{"still", "trickplay", "match", "optimize", "scan", "commercials", "dynamicrange", "still", "intros", "zzz", "artwork"} {
		if err := d.Enqueue(ctx, k, int64(i), k); err != nil {
			t.Fatal(err)
		}
	}
	claim := func(encode bool) []string {
		var out []string
		for {
			j, err := d.ClaimJob(ctx, encode)
			if err != nil {
				t.Fatal(err)
			}
			if j == nil {
				return out
			}
			out = append(out, j.Kind)
		}
	}
	if got, want := claim(false), []string{"scan", "match", "artwork", "still", "dynamicrange", "still", "zzz"}; !reflect.DeepEqual(got, want) {
		t.Errorf("general = %v, want %v", got, want)
	}
	if got, want := claim(true), []string{"commercials", "optimize", "intros", "trickplay"}; !reflect.DeepEqual(got, want) {
		t.Errorf("encode = %v, want %v", got, want)
	}
	var plan string
	rows, _ := d.sql.Query(`EXPLAIN QUERY PLAN SELECT id FROM jobs WHERE status = 'queued' AND kind = ? ORDER BY id LIMIT 1`, "still")
	for rows.Next() {
		var a, b, c int
		var detail string
		rows.Scan(&a, &b, &c, &detail)
		plan += detail
	}
	rows.Close()
	if !strings.Contains(plan, "jobs_queued") {
		t.Errorf("claim doesn't use jobs_queued: %s", plan)
	}
}

// A job queued again for later isn't claimed before its time.
func TestRetryJobLater(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	if err := d.Enqueue(ctx, "match", 1, "Match Heat"); err != nil {
		t.Fatal(err)
	}
	j, err := d.ClaimJob(ctx, false)
	if err != nil || j == nil {
		t.Fatalf("claim: %v, %v", j, err)
	}
	if err := d.RetryJobLater(ctx, j.ID, time.Hour, "network down (trying again in 1h0m0s)"); err != nil {
		t.Fatal(err)
	}
	if again, _ := d.ClaimJob(ctx, false); again != nil {
		t.Fatal("claimed before its time")
	}
	if got, _ := d.Job(ctx, j.ID); got.Status != "queued" || got.Error == "" {
		t.Fatalf("job = %+v, want queued with the reason", got)
	}
	d.sql.Exec(`UPDATE jobs SET not_before = unixepoch() - 1 WHERE id = ?`, j.ID)
	if again, _ := d.ClaimJob(ctx, false); again == nil || again.ID != j.ID || again.Attempts != 2 {
		t.Fatalf("after its time: %+v", again)
	}
}

// Recordings pages through the whole list in a stable order, and
// RecordingsCount counts what it can list.
func TestRecordingsPaging(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	for i := 0; i < 7; i++ {
		if _, err := d.sql.Exec(`INSERT INTO recordings (channel, title, start_at, end_at, status) VALUES ('2.1', ?, ?, ?, 'completed')`,
			fmt.Sprintf("Show %d", i), 1000+i, 2000+i); err != nil {
			t.Fatal(err)
		}
	}
	d.sql.Exec(`INSERT INTO recordings (channel, title, start_at, end_at, status) VALUES ('2.1', 'Cancelled', 1, 2, 'cancelled')`)
	if n, _ := d.RecordingsCount(ctx); n != 7 {
		t.Fatalf("count = %d, want 7 (cancelled left out)", n)
	}
	seen := map[int64]bool{}
	for off := 0; off < 7; off += 3 {
		page, err := d.Recordings(ctx, 3, off)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range page {
			if seen[r.ID] {
				t.Fatalf("recording %d on two pages", r.ID)
			}
			seen[r.ID] = true
		}
	}
	if len(seen) != 7 {
		t.Fatalf("pages covered %d recordings, want 7", len(seen))
	}
}

// The Manage list's one pass over files gives each title the same numbers
// as asking for that title alone.
func TestManageRowsJoinMatchesSubqueries(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	me, _ := d.SetupAdmin(ctx, "Me", "hash")
	seedLibrary(t, d, me.ID, 40)
	// Parts and editions too: copies of a few titles become a part, or an
	// Extended cut.
	var ids []int64
	rows, _ := d.sql.Query(`SELECT id FROM files WHERE role = 'copy' ORDER BY id LIMIT 6`)
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for i, id := range ids {
		if i%2 == 0 {
			if err := d.SetFileRole(ctx, id, "part", 2, ""); err != nil {
				t.Fatal(err)
			}
		} else if err := d.SetFileEdition(ctx, id, "Extended"); err != nil {
			t.Fatal(err)
		}
	}
	libs, err := d.Libraries(ctx)
	if err != nil || len(libs) == 0 {
		t.Fatalf("libraries: %v %v", libs, err)
	}
	all, err := d.ManageRows(ctx, libs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 40 {
		t.Fatalf("%d rows, want 40", len(all))
	}
	for _, r := range all {
		one, err := d.ManageRow(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(r, one) {
			t.Fatalf("item %d:\n list %+v\n  one %+v", r.ID, r, one)
		}
	}
}
