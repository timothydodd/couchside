package worker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/timothydodd/couchside/internal/probe"
	"github.com/timothydodd/couchside/internal/transcode"
)

// KindOptimize makes a browser-friendly H.264/AAC MP4 copy of a file in the
// cache. The player then direct-plays it instead of transcoding live.
const KindOptimize = "optimize" // ref: file id

func OptimizedFile(cacheDir string, fileID int64) string {
	return filepath.Join(cacheDir, "optimized", fmt.Sprintf("%d.mp4", fileID))
}

func (w *Worker) optimize(ctx context.Context, jobID, fileID int64) error {
	f, err := w.db.File(ctx, fileID)
	if err != nil {
		return err
	}
	info, err := probe.Probe(ctx, w.cfg.FFprobe, f.Path)
	if err != nil {
		return err
	}
	duration := 0.0
	if info.DurationSec != nil {
		duration = *info.DurationSec
	}
	srcH := 0
	if info.Height != nil {
		srcH = *info.Height
	}
	out := OptimizedFile(w.cfg.CacheDir, fileID)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	tmp := out + ".part.mp4"
	defer os.Remove(tmp)

	// Already-browsable H.264 that just sits in the wrong container (MKV) or
	// has the wrong audio only needs a remux: fast and lossless.
	copyVideo := info.VideoCodec == "h264" && info.EightBit420() && !info.HDR() && (srcH == 0 || srcH <= w.cfg.OptimizeHeight)
	height := transcode.OutputHeight(w.cfg.OptimizeHeight, srcH)

	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}
	var vIn, vOut []string
	if !copyVideo {
		vIn, vOut = w.enc.Video(transcode.VideoOpts{MaxHeight: height, SrcHeight: srcH,
			BitrateK: transcode.BitrateFor(height) + 2000, HDR: info.HDR(), File: true})
	}
	args = append(args, vIn...)
	args = append(args, "-i", f.Path, "-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn", "-map_chapters", "-1")
	if copyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, vOut...)
	}
	if info.AudioCodec == "aac" && info.AudioChannels <= 2 {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, transcode.AudioArgs()...)
	}
	args = append(args, "-movflags", "+faststart", "-progress", "pipe:1", "-nostats", tmp)

	cmd := exec.CommandContext(ctx, w.cfg.FFmpeg, args...)
	stderr := &strings.Builder{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// ffmpeg -progress prints key=value lines; out_time_us is position encoded.
	last := time.Time{}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok || k != "out_time_us" || duration <= 0 {
			continue
		}
		us, err := strconv.ParseFloat(v, 64)
		if err != nil || time.Since(last) < 2*time.Second {
			continue
		}
		last = time.Now()
		_ = w.db.SetJobProgress(ctx, jobID, min(0.999, us/1e6/duration))
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg: %v: %s", err, lastLine(stderr.String()))
	}
	if err := os.Rename(tmp, out); err != nil {
		return err
	}
	st, err := os.Stat(out)
	if err != nil {
		return err
	}
	outH := height
	if copyVideo {
		outH = srcH
	}
	_ = w.db.SetJobProgress(ctx, jobID, 1)
	return w.db.SetOptimized(ctx, fileID, out, st.Size(), outH)
}

// cleanupOptimized removes optimized copies whose file no longer exists
// (pruned, or replaced by a changed file) and leftovers from killed encodes.
func (w *Worker) cleanupOptimized(ctx context.Context) {
	known, err := w.db.OptimizedPaths(ctx)
	if err != nil {
		return
	}
	entries, _ := os.ReadDir(filepath.Join(w.cfg.CacheDir, "optimized"))
	for _, e := range entries {
		p := filepath.Join(w.cfg.CacheDir, "optimized", e.Name())
		if !known[p] {
			if err := os.Remove(p); err == nil {
				slog.Info("removed orphaned optimized file", "path", p)
			}
		}
	}
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

var errCancelled = errors.New("Cancelled")
