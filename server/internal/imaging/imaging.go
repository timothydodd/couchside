// Package imaging produces WebP artwork with ffmpeg: resized posters and
// frame grabs for backdrops and episode stills.
package imaging

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type FFmpeg struct{ Bin string }

// Resize converts any image ffmpeg can read into a WebP of the given width.
func (f FFmpeg) Resize(ctx context.Context, src, dst string, width int) error {
	return f.run(ctx, dst, "-i", src, "-vf", fmt.Sprintf("scale='min(%d,iw)':-2", width), "-frames:v", "1",
		"-c:v", "libwebp", "-quality", "82")
}

// FrameGrab writes one frame at the given offset as a WebP. Seeking before
// -i is a fast keyframe seek, so this stays cheap even on large remuxes.
func (f FFmpeg) FrameGrab(ctx context.Context, video, dst string, at float64, width int) error {
	// 0:V:0 is the first video stream that isn't embedded cover art.
	return f.run(ctx, dst, "-ss", strconv.FormatFloat(at, 'f', 2, 64), "-i", video, "-map", "0:V:0",
		"-vf", fmt.Sprintf("scale=%d:-2", width), "-frames:v", "1", "-an", "-sn",
		"-c:v", "libwebp", "-quality", "75")
}

// run writes to a temp file and renames, so readers never see a partial image.
func (f FFmpeg) run(ctx context.Context, dst string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	tmp := dst + ".tmp.webp"
	full := append([]string{"-y", "-v", "error", "-nostdin"}, args...)
	full = append(full, tmp)
	out, err := exec.CommandContext(ctx, f.Bin, full...).CombinedOutput()
	if err != nil {
		os.Remove(tmp)
		msg := strings.TrimSpace(string(out))
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return fmt.Errorf("ffmpeg: %v: %s", err, msg)
	}
	return os.Rename(tmp, dst)
}

// GrabOffset picks a representative moment: 20% in, skipping cold opens and
// logos, or 60s when the duration is unknown.
func GrabOffset(duration *float64, frac float64) float64 {
	if duration == nil || *duration <= 0 {
		return 60
	}
	return max(1, *duration*frac)
}
