package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// A TV show whose folder is named like an extras folder is still a show.
func TestTVShowNamedLikeExtrasFolder(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	root := filepath.Join(dir, "tv")
	show := filepath.Join(root, "Extras", "Season 1", "Extras S01E01.mkv")
	extra := filepath.Join(root, "Lost", "Featurettes", "Lost S01E01 Making Of.mkv")
	for _, p := range []string{show, extra} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	lib, _ := d.CreateLibrary(ctx, "TV", root, "tv")
	w := &Worker{db: d, cfg: config.Config{FFprobe: filepath.Join(dir, "no-ffprobe")}, wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.FileStamp(ctx, show); st == nil {
		t.Error(`the show "Extras" wasn't indexed`)
	}
	if st, _ := d.FileStamp(ctx, extra); st != nil {
		t.Error("a featurette inside a show folder was indexed as an episode")
	}
}

// The pieces of a recording whose join failed belong to that recording: they
// aren't indexed as episodes. A stray piece no recording owns is a file like
// any other.
func TestScanSkipsAFailedRecordingsParts(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	root := filepath.Join(dir, "tv")
	final := filepath.Join(root, "Ghosts", "Season 01", "Ghosts - S01E01 - Pilot.ts")
	part := filepath.Join(root, "Ghosts", "Season 01", "Ghosts - S01E01 - Pilot.part0.ts")
	stray := filepath.Join(root, "Ghosts", "Season 01", "Ghosts - S01E02 - Hello.part0.ts")
	for _, p := range []string{part, stray} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	lib, _ := d.CreateLibrary(ctx, "TV", root, "tv")
	id, _, err := d.ScheduleRecording(ctx, db.Recording{Channel: "2.1", Title: "Ghosts", StartAt: 100, EndAt: 200}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishRecording(ctx, id, "failed", final, 0, "Couldn't finish the file"); err != nil {
		t.Fatal(err)
	}
	w := &Worker{db: d, cfg: config.Config{FFprobe: filepath.Join(dir, "no-ffprobe")}, wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.FileStamp(ctx, part); st != nil {
		t.Error("a failed recording's piece was indexed as an episode")
	}
	if st, _ := d.FileStamp(ctx, stray); st == nil {
		t.Error("a piece no recording owns should still be indexed")
	}
}
