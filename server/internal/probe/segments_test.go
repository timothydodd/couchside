package probe

import (
	"fmt"
	"strings"
	"testing"
)

// packets makes ffprobe lines: one keyframe every gop seconds, 24 frames a
// second, frames of size bytes.
func packets(seconds, gop float64, size int) []byte {
	var b strings.Builder
	for i := 0; float64(i) < seconds*24; i++ {
		t := float64(i) / 24
		flags := "__"
		if i%int(gop*24) == 0 {
			flags = "K_"
		}
		fmt.Fprintf(&b, "%.6f,%d,%s\n", t, size, flags)
	}
	return []byte(b.String())
}

func TestLargestSegment(t *testing.T) {
	// Keyframes every 2s with 4s segments: two GOPs a segment.
	if got := largestSegment(packets(30, 2, 1000), 4); got != 4*24*1000 {
		t.Errorf("2s GOP = %d, want %d", got, 4*24*1000)
	}
	// Keyframes every 10s: the segment is the whole GOP (The Blob).
	if got := largestSegment(packets(30, 10, 1000), 4); got != 10*24*1000 {
		t.Errorf("10s GOP = %d, want %d", got, 10*24*1000)
	}
	// No second keyframe in the window: at least everything read.
	if got := largestSegment(packets(30, 60, 1000), 4); got != 30*24*1000 {
		t.Errorf("60s GOP = %d, want %d", got, 30*24*1000)
	}
	if got := largestSegment([]byte("N/A,12,K_\nbad\n"), 4); got != 0 {
		t.Errorf("junk = %d", got)
	}
}
