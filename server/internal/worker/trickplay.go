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
	"log/slog"
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

	frames, err := w.grabFrames(ctx, f, height, tmp)
	if err != nil {
		return err
	}
	if len(frames) == 0 {
		return fmt.Errorf("ffmpeg made no thumbnails")
	}
	// One slot per interval of the file's length (the last slot can come out
	// empty, and damaged stretches leave gaps): a slot with no frame shows
	// the one before it, which is nearer the truth than a black tile.
	count := len(frames)
	if f.DurationSec != nil && *f.DurationSec > 0 {
		count = int(math.Ceil(*f.DurationSec / trickInterval))
	}
	for len(frames) < count {
		frames = append(frames, nil)
	}
	frames = frames[:count]
	var last []byte
	for i := range frames {
		if frames[i] == nil {
			frames[i] = last
		} else {
			last = frames[i]
		}
	}
	for i := range frames { // nothing before the first frame: use the first
		if frames[i] == nil {
			frames[i] = last
		}
	}
	ix := TrickIndex{Interval: trickInterval, Width: trickWidth, Height: height, Cols: trickCols, Rows: trickRows,
		Count: count, Sheets: (count + trickCols*trickRows - 1) / (trickCols * trickRows), Size: f.Size, Mtime: f.Mtime}
	if err := writeSheets(tmp, frames, ix); err != nil {
		return fmt.Errorf("sheets: %w", err)
	}
	if err := writeBIF(tmp, frames, ix); err != nil {
		return fmt.Errorf("bif: %w", err)
	}
	b, _ := json.Marshal(ix)
	if err := os.WriteFile(filepath.Join(tmp, "index.json"), b, 0o644); err != nil {
		return err
	}
	_ = os.RemoveAll(dir)
	return os.Rename(tmp, dir)
}

// grabFrames has ffmpeg write one JPEG per trickInterval seconds of the file
// into dir ("f<N>.jpg", N from 0) and returns them in order, nil where a
// moment yielded nothing. It decodes keyframes only (a tenth of the work),
// unless that finds none: some remuxes carry keyframes the decoder won't
// take that way, and then the whole file is decoded. A broadcast recording
// can hold damaged stretches whose malformed frames make ffmpeg give up
// (its filters can't take a picture that changes shape mid-file); rather
// than lose the whole file, the run resumes a slot past where it stopped.
func (w *Worker) grabFrames(ctx context.Context, f db.File, height int, dir string) ([][]byte, error) {
	var frames [][]byte
	next := 0 // the slot the next run starts at
	keyframesOnly := true
	for attempt := 0; attempt < 24; attempt++ {
		if f.DurationSec != nil && *f.DurationSec > 0 && float64(next*trickInterval) >= *f.DurationSec {
			break
		}
		args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
		if keyframesOnly {
			args = append(args, "-skip_frame", "nokey")
		}
		if next > 0 {
			args = append(args, "-ss", fmt.Sprint(next*trickInterval))
		}
		// yuvj420p: full-range 8-bit, which the JPEG encoder wants whatever the source is.
		filter := fmt.Sprintf("fps=1/%d,scale=%d:%d,format=yuvj420p", trickInterval, trickWidth, height)
		args = append(args, "-i", f.Path, "-map", "0:v:0", "-an", "-sn", "-dn",
			"-vf", filter, "-q:v", "6", "-start_number", fmt.Sprint(next), filepath.Join(dir, "f%d.jpg"))
		out, runErr := exec.CommandContext(ctx, w.cfg.FFmpeg, args...).CombinedOutput()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		got := 0
		for {
			b, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("f%d.jpg", next+got)))
			if err != nil {
				break
			}
			for len(frames) < next+got {
				frames = append(frames, nil)
			}
			frames = append(frames, b)
			got++
		}
		if runErr == nil {
			break // the end of the file
		}
		if got == 0 && next == 0 && keyframesOnly {
			keyframesOnly = false // nothing came out: decode everything instead
			continue
		}
		if got == 0 && len(frames) == 0 {
			return nil, fmt.Errorf("ffmpeg: %v: %s", runErr, transcode.Tail(string(out), 300))
		}
		// Stopped part way (damaged data): carry on from the slot after the last one.
		slog.Warn("preview thumbnails: ffmpeg stopped part way; resuming after it", "file", filepath.Base(f.Path),
			"at", fmt.Sprintf("%ds", (next+got)*trickInterval), "said", transcode.Tail(string(out), 160))
		next = len(frames) + 1
	}
	return frames, nil
}

// writeSheets tiles the frames into dir/<n>.jpg, trickCols by trickRows per
// sheet, counted across then down; a missing frame is a black tile.
func writeSheets(dir string, frames [][]byte, ix TrickIndex) error {
	per := ix.Cols * ix.Rows
	for s := 0; s < ix.Sheets; s++ {
		sheet := image.NewRGBA(image.Rect(0, 0, ix.Width*ix.Cols, ix.Height*ix.Rows))
		for t := 0; t < per && s*per+t < len(frames); t++ {
			b := frames[s*per+t]
			if b == nil {
				continue
			}
			img, err := jpeg.Decode(bytes.NewReader(b))
			if err != nil {
				continue
			}
			at := image.Pt(t%ix.Cols*ix.Width, t/ix.Cols*ix.Height)
			draw.Draw(sheet, image.Rectangle{Min: at, Max: at.Add(image.Pt(ix.Width, ix.Height))}, img, image.Point{}, draw.Src)
		}
		out, err := os.Create(filepath.Join(dir, fmt.Sprintf("%d.jpg", s)))
		if err != nil {
			return err
		}
		err = jpeg.Encode(out, sheet, &jpeg.Options{Quality: 80})
		out.Close()
		if err != nil {
			return err
		}
	}
	// The single frames were only input.
	for i := range frames {
		_ = os.Remove(filepath.Join(dir, fmt.Sprintf("f%d.jpg", i)))
	}
	return nil
}

// writeBIF packs the frames into dir/index.bif, the Roku's trick-play
// format: a 64-byte header, a table of (timestamp, offset) pairs ending with
// 0xffffffff, then the JPEG frames one after another. A missing frame is a
// black one, so the table stays regular.
func writeBIF(dir string, in [][]byte, ix TrickIndex) error {
	var blank []byte
	frames := make([][]byte, 0, len(in))
	for _, f := range in {
		if f == nil {
			if blank == nil {
				var buf bytes.Buffer
				if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, ix.Width, ix.Height)), &jpeg.Options{Quality: 50}); err != nil {
					return err
				}
				blank = buf.Bytes()
			}
			f = blank
		}
		frames = append(frames, f)
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
