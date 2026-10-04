package worker

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// An episode indexed without a still (before stills existed, or with the
// cache wiped) gets one queued by the next scan's catch-up, as do preview
// thumbnails in a library that has them on. A file whose job failed isn't
// queued again while that failure is in Activity.
func TestCatchUpQueuesWhatFilesLack(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	libID, _ := d.CreateLibrary(ctx, "Shows", dir, "tv")
	item, _, _ := d.EnsureItem(ctx, libID, "series", "Show", 0)
	ep, err := d.EnsureEpisode(ctx, item, 1, 1, "", "")
	if err != nil {
		t.Fatal(err)
	}
	fid, err := d.UpsertFile(ctx, db.File{LibraryID: libID, MediaItemID: item, EpisodeID: &ep, Path: filepath.Join(dir, "Show S01E01.mkv"), Size: 1, Mtime: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	w := &Worker{db: d, cfg: config.Config{CacheDir: filepath.Join(dir, "cache")}, wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}
	queued := func() map[string]int {
		t.Helper()
		jobs, err := d.RecentJobs(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		n := map[string]int{}
		for _, j := range jobs {
			if j.Status == "queued" && j.RefID == fid {
				n[j.Kind]++
			}
		}
		return n
	}
	catchUp := func() {
		t.Helper()
		lib, err := d.Library(ctx, libID)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.catchUp(ctx, lib); err != nil {
			t.Fatal(err)
		}
	}

	catchUp()
	if q := queued(); q[KindStill] != 1 || q[KindTrickplay] != 0 {
		t.Fatalf("queued %v, want one still and no previews while they're off", q)
	}
	if err := d.SetLibraryTrickplay(ctx, libID, true); err != nil {
		t.Fatal(err)
	}
	catchUp()
	if q := queued(); q[KindStill] != 1 || q[KindTrickplay] != 1 {
		t.Fatalf("queued %v, want one still and one preview job", q)
	}

	// Both fail: the next scans leave them alone.
	for range 2 {
		j, err := d.ClaimJob(ctx, false)
		if err != nil || j == nil {
			j, err = d.ClaimJob(ctx, true)
		}
		if err != nil || j == nil {
			t.Fatalf("no job to claim: %v", err)
		}
		if err := d.FinishJob(ctx, j.ID, errors.New("boom")); err != nil {
			t.Fatal(err)
		}
	}
	catchUp()
	if q := queued(); len(q) != 0 {
		t.Fatalf("failed jobs were queued again: %v", q)
	}
	// Switching previews on again is a request to retry.
	if n, err := w.QueueTrickplay(ctx, libID); err != nil || n != 1 {
		t.Fatalf("QueueTrickplay queued %d, %v", n, err)
	}

	// A file that has its still isn't queued.
	if err := d.ClearFinishedJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.SetFileStill(ctx, fid, true); err != nil {
		t.Fatal(err)
	}
	if files, _ := d.FilesNeedingStills(ctx, libID); len(files) != 0 {
		t.Fatalf("%d files need stills, want 0", len(files))
	}
}
