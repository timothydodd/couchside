package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateLibraryKeepsFilesAndHistory(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := WithProfile(context.Background(), 1)

	lib, _ := d.CreateLibrary(ctx, "Films", "/media/Películas", "movies")
	other, _ := d.CreateLibrary(ctx, "TV", "/media/TV", "tv")
	item, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat", 1995)
	fid, err := d.UpsertFile(ctx, File{LibraryID: lib, MediaItemID: item, Path: "/media/Películas/Heat (1995)/Heat.mkv", Size: 1, Mtime: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SaveProgress(ctx, fid, 600, 6000); err != nil {
		t.Fatal(err)
	}

	// Rename only.
	if err := d.UpdateLibrary(ctx, lib, "Movies", "/media/Películas"); err != nil {
		t.Fatal(err)
	}
	if l, _ := d.Library(ctx, lib); l.Name != "Movies" {
		t.Fatalf("name = %q", l.Name)
	}

	// New folder: the file is re-pointed and keeps its id and progress.
	if err := d.UpdateLibrary(ctx, lib, "Movies", "/media/Movies"); err != nil {
		t.Fatal(err)
	}
	f, err := d.File(ctx, fid)
	if err != nil {
		t.Fatal(err)
	}
	if f.Path != "/media/Movies/Heat (1995)/Heat.mkv" || f.PositionSec != 600 {
		t.Fatalf("after move: path %q, position %v", f.Path, f.PositionSec)
	}

	// Another library's folder is refused, and nothing changes.
	if err := d.UpdateLibrary(ctx, lib, "Movies", "/media/TV"); !errors.Is(err, ErrPathInUse) {
		t.Fatalf("taken folder err = %v", err)
	}
	if l, _ := d.Library(ctx, lib); l.Path != "/media/Movies" {
		t.Fatalf("path after refused update = %q", l.Path)
	}
	if err := d.UpdateLibrary(ctx, 999, "x", "/x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing library err = %v", err)
	}
	_ = other
}

func TestPruneJobsAndCache(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	_ = d.Enqueue(ctx, "scan", 1, "old")
	_ = d.Enqueue(ctx, "scan", 2, "queued")
	j, _ := d.ClaimJob(ctx, false)
	_ = d.FinishJob(ctx, j.ID, nil)
	if n, err := d.PruneJobs(ctx, time.Now().Add(time.Hour).Unix()); err != nil || n != 1 {
		t.Fatalf("PruneJobs = %d, %v; want the finished job only", n, err)
	}
	_ = d.CachePut(ctx, "tmdb", "k", []byte("{}"))
	if n, _ := d.PruneCache(ctx, time.Now().Add(-time.Hour).Unix()); n != 0 {
		t.Fatal("a fresh cache row was pruned")
	}
	if n, _ := d.PruneCache(ctx, time.Now().Add(time.Hour).Unix()); n != 1 {
		t.Fatal("an old cache row was kept")
	}
}
