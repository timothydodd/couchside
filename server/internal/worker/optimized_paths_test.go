package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCache(t *testing.T) {
	cache := filepath.Join(string(filepath.Separator)+"srv", "cache")
	if got := ResolveCache(cache, "optimized/7.mp4"); got != filepath.Join(cache, "optimized", "7.mp4") {
		t.Errorf("relative: %s", got)
	}
	abs := filepath.Join(string(filepath.Separator)+"old", "optimized", "7.mp4")
	if got := ResolveCache(cache, abs); got != abs {
		t.Errorf("absolute kept: %s", got)
	}
	if got := OptimizedFile(cache, 9); got != filepath.Join(cache, "optimized", "9.mp4") {
		t.Errorf("OptimizedFile = %s", got)
	}
}

// Rows from before 0.19 hold absolute paths: one under today's cache, or
// under a cache folder that has since moved, becomes relative; a copy whose
// row is gone is removed; one on record stays.
func TestCleanupOptimizedRelativePaths(t *testing.T) {
	w, d, lib, root := scanFixture(t)
	ctx := context.Background()
	for _, n := range []string{"Heat (1995).mkv", "Alien (1979).mkv"} {
		writeVideo(t, filepath.Join(root, n))
	}
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	ids := indexed(t, d, filepath.Join(root, "Heat (1995).mkv"), filepath.Join(root, "Alien (1979).mkv"))
	underCache := OptimizedFile(w.cfg.CacheDir, ids[0])
	moved := OptimizedFile(w.cfg.CacheDir, ids[1])
	stray := OptimizedFile(w.cfg.CacheDir, 999)
	for _, p := range []string{underCache, moved, stray} {
		writeVideo(t, p)
	}
	if err := d.SetOptimized(ctx, ids[0], underCache, 1, 720); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(t.TempDir(), "old-cache", "optimized", filepath.Base(moved))
	if err := d.SetOptimized(ctx, ids[1], gone, 1, 720); err != nil {
		t.Fatal(err)
	}
	w.cleanupOptimized(ctx)
	paths, err := d.OptimizedPaths(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if paths[ids[0]] != OptimizedRel(ids[0]) || paths[ids[1]] != OptimizedRel(ids[1]) {
		t.Errorf("stored paths = %v, want both relative", paths)
	}
	for _, p := range []string{underCache, moved} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed", p)
		}
	}
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Errorf("stray copy wasn't removed")
	}
}
