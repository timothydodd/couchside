package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

func TestScanSummary(t *testing.T) {
	var none skipLog
	if got := scanSummary(0, 0, 0, 0, &none); got != "No changes." {
		t.Errorf("nothing = %q", got)
	}
	if got := scanSummary(0, 3, 0, 0, &none); got != "3 changed." {
		t.Errorf("changed only = %q", got)
	}
	var s skipLog
	for _, n := range []string{"a.mp4", "b.mp4", "c.mp4", "d.mp4"} {
		s.add(skipNoEpisode, n)
	}
	s.add(skipSample, "Film/sample.mkv")
	want := "2 added, 1 removed. Skipped 4: no season and episode in the name (a.mp4, b.mp4, c.mp4 and 1 more). Skipped 1: sample clips (Film/sample.mkv)."
	if got := scanSummary(2, 0, 1, 0, &s); got != want {
		t.Errorf("summary = %q\nwant      %q", got, want)
	}
}

// A TV library's root holding films (no season and episode) says so in the
// scan's result, which Activity shows.
func TestScanResultNamesSkippedFiles(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	root := filepath.Join(dir, "TV")
	os.MkdirAll(filepath.Join(root, "Show (1962)", "Season 01"), 0o755)
	for _, n := range []string{"Porky's Railroad (1937).mp4", "Railroad Rhythm (1937).mp4", "Show (1962)/Season 01/Show - S01E01 - Pilot.mp4"} {
		os.WriteFile(filepath.Join(root, n), []byte("x"), 0o644)
	}
	lib, _ := d.CreateLibrary(ctx, "TV", root, "tv")
	w := &Worker{db: d, cfg: config.Config{FFprobe: filepath.Join(dir, "no-ffprobe")}, wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}
	got, err := w.scanLibrary(ctx, lib)
	if err != nil {
		t.Fatal(err)
	}
	want := "1 added. Skipped 2: no season and episode in the name (Porky's Railroad (1937).mp4, Railroad Rhythm (1937).mp4)."
	if got != want {
		t.Fatalf("result = %q\nwant     %q", got, want)
	}
}
