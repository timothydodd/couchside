package api

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// Library counts are kept for countsTTL, and dropped when asked.
func TestCachedCounts(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, config.Config{DataDir: dir}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if c, _ := s.cachedCounts(ctx); c.Libraries != 0 {
		t.Fatalf("libraries = %d", c.Libraries)
	}
	if _, err := d.CreateLibrary(ctx, "Films", "/films", "movies"); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.cachedCounts(ctx); c.Libraries != 0 {
		t.Fatal("counted again inside the TTL")
	}
	s.dropCounts()
	if c, _ := s.cachedCounts(ctx); c.Libraries != 1 {
		t.Fatal("didn't count again after dropCounts")
	}
	d.CreateLibrary(ctx, "TV", "/tv", "tv")
	old := countsTTL
	countsTTL = 0
	t.Cleanup(func() { countsTTL = old })
	time.Sleep(time.Millisecond)
	if c, _ := s.cachedCounts(ctx); c.Libraries != 2 {
		t.Fatal("didn't count again once the TTL passed")
	}
}
