package worker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

func TestParseEDL(t *testing.T) {
	in := "610.21\t790.50\t0\n" +
		"0.00\t32.10\t0\n" +
		"790.80\t850.00\t3\n" + // touches the previous break: merged
		"garbage line\n" +
		"900\t880\t0\n" + // end before start: dropped
		"-1.5\t2\t0\n"
	got := parseEDL(in)
	want := []db.Segment{{Start: 0, End: 32.1}, {Start: 610.21, End: 850}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEDL = %v, want %v", got, want)
	}
	if got := parseEDL(""); len(got) != 0 || got == nil {
		t.Fatalf("empty EDL = %#v, want empty non-nil slice", got)
	}
}

// With COUCHSIDE_COMSKIP_INI set and a cache that has never run comskip, the
// work folder didn't exist and every job failed before starting.
func TestCommercialsWithACustomIniOnAFreshCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as comskip")
	}
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	comskip := filepath.Join(dir, "comskip")
	script := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in --output=*) out=\"${a#--output=}\";; esac; done\n" +
		"printf '10\\t20\\t0\\n' > \"$out/couchside.edl\"\n"
	if err := os.WriteFile(comskip, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ini := filepath.Join(dir, "my.ini")
	if err := os.WriteFile(ini, []byte("detect_method=43\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib, _ := d.CreateLibrary(ctx, "TV", dir, "tv")
	item, _, _ := d.EnsureItem(ctx, lib, "series", "Show", 0)
	ep, _ := d.EnsureEpisode(ctx, item, 1, 1, "", "")
	rec := filepath.Join(dir, "Show - S01E01.ts")
	if err := os.WriteFile(rec, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fid, err := d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, EpisodeID: &ep, Path: rec, Size: 1, Mtime: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	w := &Worker{db: d, cfg: config.Config{CacheDir: filepath.Join(dir, "cache"), ComskipINI: ini}, comskip: comskip,
		wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}
	if err := w.commercials(ctx, 0, fid); err != nil {
		t.Fatalf("commercials with a custom ini on an empty cache: %v", err)
	}
	segs, done, err := d.Commercials(ctx, fid)
	if err != nil || !done || len(segs) != 1 || segs[0].Start != 10 {
		t.Fatalf("result: %v done=%v err=%v", segs, done, err)
	}
}
