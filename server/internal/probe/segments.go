package probe

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// LargestSegment is the size in bytes of the biggest HLS segment the video
// would make if copied as is, over length seconds from `from`. A copy can only
// be cut at the file's own keyframes: ffmpeg's HLS muxer ends a segment at
// the first keyframe segDur seconds or more after it began, so a film with
// keyframes 10s apart at 40 Mbit/s makes 50 MB segments whatever segDur is.
// Only video packets count (audio adds little). When the window holds no
// whole segment (keyframes further apart than length), everything read is
// the answer, since a segment is at least that big.
func LargestSegment(ctx context.Context, bin, path string, from, length float64, segDur int) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	interval := fmt.Sprintf("%.3f%%+%.3f", from, length)
	out, err := exec.CommandContext(ctx, bin, "-v", "error", "-select_streams", "v:0", "-read_intervals", interval,
		"-show_entries", "packet=pts_time,size,flags", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe packets: %w", err)
	}
	return largestSegment(out, float64(segDur)), nil
}

// largestSegment reads ffprobe's "pts_time,size,flags" lines.
func largestSegment(csv []byte, segDur float64) int64 {
	var biggest, cur int64
	start := -1.0
	whole := false
	sc := bufio.NewScanner(bytes.NewReader(csv))
	for sc.Scan() {
		f := strings.Split(strings.TrimSpace(sc.Text()), ",")
		if len(f) < 3 {
			continue
		}
		size, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		pts, ptsErr := strconv.ParseFloat(f[0], 64)
		if strings.Contains(f[2], "K") && ptsErr == nil {
			switch {
			case start < 0:
				start, cur = pts, 0 // the first keyframe starts the first segment
			case pts-start >= segDur:
				biggest, whole = max(biggest, cur), true
				start, cur = pts, 0
			}
		}
		if start >= 0 {
			cur += size
		}
	}
	if !whole {
		return cur
	}
	return biggest
}
