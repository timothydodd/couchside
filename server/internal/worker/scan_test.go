package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// scanFixture is a worker on a temp DB with one movie library. ffprobe is a
// missing binary, so files are indexed as unreadable without probing.
func scanFixture(t *testing.T) (*Worker, *db.DB, int64, string) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	root := filepath.Join(dir, "media")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	lib, err := d.CreateLibrary(context.Background(), "Films", root, "movies")
	if err != nil {
		t.Fatal(err)
	}
	w := &Worker{db: d, cfg: config.Config{FFprobe: filepath.Join(dir, "no-ffprobe")},
		wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}
	return w, d, lib, root
}

func writeVideo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// indexed reports which of paths have a file row.
func indexed(t *testing.T, d *db.DB, paths ...string) (ids []int64) {
	t.Helper()
	for _, p := range paths {
		st, err := d.FileStamp(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		if st == nil {
			ids = append(ids, 0)
		} else {
			ids = append(ids, st.ID)
		}
	}
	return ids
}

// backdate makes files look seen by an earlier scan, since last_seen is in
// whole seconds and the scans in a test run within one.
func backdate(t *testing.T, d *db.DB, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		if err := d.TouchFile(context.Background(), id, 1); err != nil {
			t.Fatal(err)
		}
	}
}

func allIndexed(ids []int64) bool {
	for _, id := range ids {
		if id == 0 {
			return false
		}
	}
	return true
}

func TestScanEmptyFolderRemovesNothing(t *testing.T) {
	w, d, lib, root := scanFixture(t)
	ctx := db.WithProfile(context.Background(), 1)
	paths := []string{filepath.Join(root, "Heat (1995).mkv"), filepath.Join(root, "Alien (1979).mkv")}
	for _, p := range paths {
		writeVideo(t, p)
	}
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	ids := indexed(t, d, paths...)
	if !allIndexed(ids) {
		t.Fatalf("first scan indexed %v", ids)
	}
	if err := d.SaveProgress(ctx, ids[0], 600, 6000); err != nil {
		t.Fatal(err)
	}
	if err := d.MarkLibraryScanned(ctx, lib, 1); err != nil {
		t.Fatal(err)
	}
	backdate(t, d, ids...)

	// The share went away: the mount point is an empty folder.
	for _, p := range paths {
		os.Remove(p)
	}
	err := w.scan(ctx, lib)
	if err == nil || !strings.Contains(err.Error(), "Nothing was removed") {
		t.Fatalf("scan of an empty folder: err = %v", err)
	}
	if got := indexed(t, d, paths...); !allIndexed(got) {
		t.Fatalf("files after empty scan = %v", got)
	}
	if f, err := d.File(ctx, ids[0]); err != nil || f.PositionSec != 600 {
		t.Fatalf("watch state after empty scan: %+v, %v", f, err)
	}
	if l, _ := d.Library(ctx, lib); l.LastScanAt == nil || *l.LastScanAt != 1 {
		t.Fatalf("failed scan marked the library scanned: %v", l.LastScanAt)
	}
}

func TestScanUnreadableFolderRemovesNothing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable folders")
	}
	w, d, lib, root := scanFixture(t)
	ctx := context.Background()
	heat, alien := filepath.Join(root, "a", "Heat (1995).mkv"), filepath.Join(root, "b", "Alien (1979).mkv")
	writeVideo(t, heat)
	writeVideo(t, alien)
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	backdate(t, d, indexed(t, d, heat, alien)...)
	b := filepath.Join(root, "b")
	if err := os.Chmod(b, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(b, 0o755) })
	if _, err := os.ReadDir(b); err == nil {
		t.Skip("this filesystem ignores permissions")
	}
	// A new file elsewhere is still indexed.
	ran := filepath.Join(root, "a", "Ran (1985).mkv")
	writeVideo(t, ran)
	if err := d.MarkLibraryScanned(ctx, lib, 1); err != nil {
		t.Fatal(err)
	}

	err := w.scan(ctx, lib)
	if err == nil || !strings.Contains(err.Error(), "nothing was removed") {
		t.Fatalf("scan with an unreadable folder: err = %v", err)
	}
	if got := indexed(t, d, heat, alien, ran); !allIndexed(got) {
		t.Fatalf("files after partial scan = %v, want all three", got)
	}
	if l, _ := d.Library(ctx, lib); l.LastScanAt == nil || *l.LastScanAt != 1 {
		t.Fatalf("incomplete scan marked the library scanned: %v", l.LastScanAt)
	}
}

func TestScanPrunesDeletedFile(t *testing.T) {
	w, d, lib, root := scanFixture(t)
	ctx := context.Background()
	heat, alien := filepath.Join(root, "Heat (1995).mkv"), filepath.Join(root, "Alien (1979).mkv")
	writeVideo(t, heat)
	writeVideo(t, alien)
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	backdate(t, d, indexed(t, d, heat, alien)...)
	os.Remove(alien)
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if got := indexed(t, d, heat, alien); got[0] == 0 || got[1] != 0 {
		t.Fatalf("after delete: heat %d, alien %d; want only alien pruned", got[0], got[1])
	}
}
