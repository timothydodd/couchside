package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// One library's prune mustn't tidy away a show another library's scan has
// just made but not yet written a file for: the prune waits for HoldItems.
// Before the hold, the second UpsertFile failed with a foreign key error.
func TestPruneWaitsForHeldItems(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	movies, _ := d.CreateLibrary(ctx, "Movies", filepath.Join(dir, "movies"), "movies")
	tv, _ := d.CreateLibrary(ctx, "TV", filepath.Join(dir, "tv"), "tv")

	release := d.HoldItems()
	show, _, err := d.EnsureItem(ctx, tv, "series", "Severance", 2022)
	if err != nil {
		t.Fatal(err)
	}
	ep, err := d.EnsureEpisode(ctx, show, 1, 1, "Good News About Hell", "")
	if err != nil {
		t.Fatal(err)
	}

	pruned := make(chan error, 1)
	go func() {
		_, err := d.PruneLibrary(ctx, movies, time.Now().Unix())
		pruned <- err
	}()
	select {
	case err := <-pruned:
		t.Fatalf("prune ran while a scan held its new items: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if _, err := d.UpsertFile(ctx, File{LibraryID: tv, MediaItemID: show, EpisodeID: &ep, Path: filepath.Join(dir, "tv", "s01e01.mkv"), Size: 1, Mtime: 1}, 1); err != nil {
		t.Fatalf("writing the file after making its show: %v", err)
	}
	release()
	if err := <-pruned; err != nil {
		t.Fatal(err)
	}
	if _, err := d.Item(ctx, show); err != nil {
		t.Fatalf("the show went with the other library's prune: %v", err)
	}
}
