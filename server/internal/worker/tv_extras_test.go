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
