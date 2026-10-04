package worker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/transcode"
)

// Seek-bar preview thumbnails ("trickplay"): one small frame every ten
// seconds of a file, tiled into JPEG sheets for the web player, and packed
// into a BIF file for the Roku, which shows it by itself while scrubbing.
// Made by a job in the encode pool, after everything else there, for files
// in libraries that have it switched on. It reads the whole file once (only
// keyframes are decoded), which over a network share takes minutes for a
// large film: hence a choice per library, not a default.
const (
	KindTrickplay = "trickplay" // ref: file id

	trickInterval = 10  // seconds between frames
	trickWidth    = 320 // pixels; the Roku's HD BIF width
	trickCols     = 10
	trickRows     = 10
)

// TrickIndex describes a file's thumbnails. Frame n (from 0) shows time
// n*Interval, and is tile n%(Cols*Rows) of sheet n/(Cols*Rows), counted
// across then down.
type TrickIndex struct {
	Interval int `json:"interval"`
	Width    int `json:"width"`
	Height   int `json:"height"`
	Cols     int `json:"cols"`
	Rows     int `json:"rows"`
	Count    int `json:"count"`  // frames
	Sheets   int `json:"sheets"` // sheet files, named 0.jpg, 1.jpg, …
	// The file these were made from: thumbnails of a file that has since
	// changed aren't used.
	Size  int64 `json:"size"`
	Mtime int64 `json:"mtime"`
}

// TrickDir is where a file's thumbnails live.
func TrickDir(cacheDir string, fileID int64) string {
	return filepath.Join(cacheDir, "files", fmt.Sprint(fileID), "trick")
}

// Trickplay returns a file's thumbnail index, or nil when it has none made
// from the file as it is now.
func Trickplay(cacheDir string, f db.File) *TrickIndex {
	b, err := os.ReadFile(filepath.Join(TrickDir(cacheDir, f.ID), "index.json"))
	if err != nil {
		return nil
	}
	var ix TrickIndex
	if json.Unmarshal(b, &ix) != nil || ix.Count == 0 || ix.Size != f.Size || ix.Mtime != f.Mtime {
		return nil
	}
	return &ix
}

// QueueTrickplay queues thumbnails for a library's files that lack them, and
// reports how many. It's what switching the library's setting on does, so
// files that failed before are tried again.
func (w *Worker) QueueTrickplay(ctx context.Context, libraryID int64) (int, error) {
	return w.queueTrickplay(ctx, libraryID, true)
}

func (w *Worker) queueTrickplay(ctx context.Context, libraryID int64, retryFailed bool) (int, error) {
	files, err := w.db.LibraryFiles(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	failed := map[int64]bool{}
	if !retryFailed {
		if failed, err = w.db.FailedJobRefs(ctx, KindTrickplay); err != nil {
			return 0, err
		}
	}
	n := 0
	for _, f := range files {
		if f.Problem != "" || failed[f.ID] || Trickplay(w.cfg.CacheDir, f) != nil {
			continue
		}
		if err := w.db.Enqueue(ctx, KindTrickplay, f.ID, "Preview thumbnails "+filepath.Base(f.Path)); err != nil {
			return n, err
		}
		n++
	}
	if n > 0 {
		w.Wake()
	}
	return n, nil
}

func (w *Worker) trickplay(ctx context.Context, fileID int64) error {
	f, err := w.db.File(ctx, fileID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil // pruned before we got to it
		}
		return err
	}
	if f.Problem != "" {
		return fmt.Errorf("the file can't be read (%s)", f.Problem)
	}
	if Trickplay(w.cfg.CacheDir, f) != nil {
		return nil
	}
	// Tile height from the picture's shape, even, as ffmpeg wants.
	height := 180
	if f.Width != nil && f.Height != nil && *f.Width > 0 && *f.Height > 0 {
		height = max(2, int(math.Round(float64(trickWidth)*float64(*f.Height)/float64(*f.Width)/2))*2)
	}
	dir := TrickDir(w.cfg.CacheDir, fileID)
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	// Keyframes only: a tenth of the decoding, and a frame near each tenth
	// second is all a preview needs.
	filter := fmt.Sprintf("fps=1/%d,scale=%d:%d,tile=%dx%d", trickInterval, trickWidth, height, trickCols, trickRows)
	cmd := exec.CommandContext(ctx, w.cfg.FFmpeg, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-skip_frame", "nokey", "-i", f.Path, "-map", "0:v:0", "-an", "-sn", "-dn",
		"-vf", filter, "-q:v", "6", "-start_number", "0", filepath.Join(tmp, "%d.jpg"))
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg: %v: %s", err, transcode.Tail(string(out), 300))
	}
	sheets := 0
	for {
		if _, err := os.Stat(filepath.Join(tmp, fmt.Sprintf("%d.jpg", sheets))); err != nil {
			break
		}
		sheets++
	}
	if sheets == 0 {
		return fmt.Errorf("ffmpeg made no thumbnails")
	}
	// The last sheet is padded with empty tiles: count frames from the length.
	count := sheets * trickCols * trickRows
	if f.DurationSec != nil && *f.DurationSec > 0 {
		count = min(count, int(math.Ceil(*f.DurationSec/trickInterval)))
	}
	ix := TrickIndex{Interval: trickInterval, Width: trickWidth, Height: height, Cols: trickCols, Rows: trickRows,
		Count: count, Sheets: sheets, Size: f.Size, Mtime: f.Mtime}
	if err := writeBIF(tmp, ix); err != nil {
		return fmt.Errorf("bif: %w", err)
	}
	b, _ := json.Marshal(ix)
	if err := os.WriteFile(filepath.Join(tmp, "index.json"), b, 0o644); err != nil {
		return err
	}
	_ = os.RemoveAll(dir)
	return os.Rename(tmp, dir)
}

// writeBIF packs the frames into dir/index.bif, the Roku's trick-play
// format: a 64-byte header, a table of (timestamp, offset) pairs ending with
// 0xffffffff, then the JPEG frames one after another.
func writeBIF(dir string, ix TrickIndex) error {
	frames := make([][]byte, 0, ix.Count)
	for s := 0; s < ix.Sheets && len(frames) < ix.Count; s++ {
		f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%d.jpg", s)))
		if err != nil {
			return err
		}
		sheet, err := jpeg.Decode(f)
		f.Close()
		if err != nil {
			return err
		}
		for t := 0; t < ix.Cols*ix.Rows && len(frames) < ix.Count; t++ {
			at := image.Pt(t%ix.Cols*ix.Width, t/ix.Cols*ix.Height)
			tile := image.NewRGBA(image.Rect(0, 0, ix.Width, ix.Height))
			draw.Draw(tile, tile.Bounds(), sheet, at, draw.Src)
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, tile, &jpeg.Options{Quality: 70}); err != nil {
				return err
			}
			frames = append(frames, buf.Bytes())
		}
	}
	var out bytes.Buffer
	out.Write([]byte{0x89, 'B', 'I', 'F', 0x0d, 0x0a, 0x1a, 0x0a})
	le := func(v uint32) { _ = binary.Write(&out, binary.LittleEndian, v) }
	le(0)                          // version
	le(uint32(len(frames)))        // number of images
	le(uint32(ix.Interval * 1000)) // milliseconds between them
	out.Write(make([]byte, 64-out.Len()))
	offset := uint32(64 + 8*(len(frames)+1))
	for i, f := range frames {
		le(uint32(i))
		le(offset)
		offset += uint32(len(f))
	}
	le(0xffffffff)
	le(offset)
	for _, f := range frames {
		out.Write(f)
	}
	return os.WriteFile(filepath.Join(dir, "index.bif"), out.Bytes(), 0o644)
}

// TrickSheetName reports whether name is one of a file's sheet files.
func TrickSheetName(name string) bool {
	n := strings.TrimSuffix(name, ".jpg")
	if n == name || n == "" || len(n) > 5 {
		return false
	}
	for _, c := range n {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
