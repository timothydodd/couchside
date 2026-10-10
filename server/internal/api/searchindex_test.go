package api

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/db"
)

// Searches are answered from the current snapshot at once; an old one is
// rebuilt in the background and swapped in, and concurrent searches never
// see a missing index.
func TestSearchIndexServesWhileRebuilding(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	lib, _ := d.CreateLibrary(ctx, "Films", "/films", "movies")
	item, _, _ := d.EnsureItem(ctx, lib, "movie", "Heat", 1995)
	d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: "/films/Heat.mkv", Size: 1, Mtime: 1}, 1)
	var x searchIndex
	docs, err := x.get(ctx, d, false)
	if err != nil || len(docs) == 0 {
		t.Fatalf("first build: %d docs, %v", len(docs), err)
	}
	before := len(docs)
	alien, _, _ := d.EnsureItem(ctx, lib, "movie", "Alien", 1979)
	d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: alien, Path: "/films/Alien.mkv", Size: 1, Mtime: 1}, 1)
	if docs, _ = x.get(ctx, d, false); len(docs) != before {
		t.Fatal("a fresh snapshot was rebuilt inside the search")
	}
	old := *x.cur.Load()
	old.built = time.Now().Add(-2 * indexTTL)
	x.cur.Store(&old)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if docs, err := x.get(ctx, d, false); err != nil || len(docs) == 0 {
				t.Errorf("search during a rebuild: %d docs, %v", len(docs), err)
			}
		}()
	}
	wg.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := x.cur.Load(); len(s.docs) > before && !x.building.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the background rebuild never brought the new title in")
}
