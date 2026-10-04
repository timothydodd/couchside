package worker

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// A 95-second clip gets ten frames on one sheet, an index tied to the file,
// and a BIF the Roku can read. Uses the real ffmpeg.
func TestTrickplayWithFFmpeg(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "Film (2020).mkv")
	gen := exec.Command(ff, "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=640x272:rate=24", "-t", "95",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", clip)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("making a test clip: %v %s", err, out)
	}
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	lib, _ := d.CreateLibrary(ctx, "Films", dir, "movies")
	item, _, _ := d.EnsureItem(ctx, lib, "movie", "Film", 2020)
	st, _ := os.Stat(clip)
	dur, wd, ht := 95.0, 640, 272
	fid, err := d.UpsertFile(ctx, db.File{LibraryID: lib, MediaItemID: item, Path: clip, Size: st.Size(), Mtime: st.ModTime().Unix(),
		DurationSec: &dur, Width: &wd, Height: &ht, VideoCodec: "h264"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	w := &Worker{db: d, cfg: config.Config{CacheDir: filepath.Join(dir, "cache"), FFmpeg: ff}, wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}

	if n, err := w.QueueTrickplay(ctx, lib); err != nil || n != 1 {
		t.Fatalf("queued %d, %v", n, err)
	}
	if err := w.trickplay(ctx, fid); err != nil {
		t.Fatal(err)
	}
	f, _ := d.File(ctx, fid)
	ix := Trickplay(w.cfg.CacheDir, f)
	if ix == nil {
		t.Fatal("no index after the job")
	}
	if ix.Count != 10 || ix.Sheets != 1 || ix.Width != 320 || ix.Height != 136 || ix.Interval != 10 {
		t.Fatalf("index = %+v", *ix)
	}
	sheet, err := os.Open(filepath.Join(TrickDir(w.cfg.CacheDir, fid), "0.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(sheet)
	sheet.Close()
	if err != nil || cfg.Width != 3200 || cfg.Height != 1360 {
		t.Fatalf("sheet is %dx%d (%v), want 3200x1360", cfg.Width, cfg.Height, err)
	}

	bif, err := os.ReadFile(filepath.Join(TrickDir(w.cfg.CacheDir, fid), "index.bif"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(bif, []byte{0x89, 'B', 'I', 'F', 0x0d, 0x0a, 0x1a, 0x0a}) {
		t.Fatal("BIF magic is wrong")
	}
	u32 := func(at int) uint32 { return binary.LittleEndian.Uint32(bif[at:]) }
	if u32(12) != 10 || u32(16) != 10000 {
		t.Fatalf("BIF header: %d images every %dms", u32(12), u32(16))
	}
	first, second := u32(64+4), u32(64+12)
	if first != 64+8*11 || second <= first {
		t.Fatalf("BIF index: first frame at %d, second at %d", first, second)
	}
	if u32(64+8*10) != 0xffffffff || int(u32(64+8*10+4)) != len(bif) {
		t.Fatal("BIF index doesn't end at the file's end")
	}
	if _, err := jpeg.Decode(bytes.NewReader(bif[first:second])); err != nil {
		t.Fatalf("the first BIF frame isn't a JPEG: %v", err)
	}

	// Already made: nothing to queue. A changed file: made again.
	if n, _ := w.QueueTrickplay(ctx, lib); n != 0 {
		t.Fatalf("queued %d for a file that has thumbnails", n)
	}
	f.Size++
	if Trickplay(w.cfg.CacheDir, f) != nil {
		t.Fatal("thumbnails of a file that changed were used")
	}
}

func TestTrickSheetName(t *testing.T) {
	for name, want := range map[string]bool{"0.jpg": true, "12.jpg": true, "index.bif": false, "../0.jpg": false, ".jpg": false, "a.jpg": false, "0.jpg.tmp": false} {
		if got := TrickSheetName(name); got != want {
			t.Errorf("TrickSheetName(%q) = %v", name, got)
		}
	}
}
