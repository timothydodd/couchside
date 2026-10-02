package transcode

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A restart in the middle of an MPEG-2 broadcast recording must put video
// in its first segment from the segment's start. MPEG-TS has no index, so a
// plain seek starts video at the next keyframe after it, seconds late (on
// VAAPI, after the whole first segment), and hls.js stalls on an audio-only
// segment followed by one with video.
func TestRestartMidRecordingHasVideoFromTheStart(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "rec.ts")
	// Broadcast-like: interlaced MPEG-2 with a long GOP, AC-3 audio, and
	// timestamps that don't start at zero.
	gen := exec.Command(ff, "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30000/1001", "-f", "lavfi",
		"-i", "sine=frequency=440", "-t", "40", "-c:v", "mpeg2video", "-flags", "+ilme+ildct", "-g", "90", "-bf", "2",
		"-c:a", "ac3", "-output_ts_offset", "1.5", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("making a test recording: %v %s", err, out)
	}
	m, err := NewManager(Encoder{FFmpeg: ff, HW: "none"}, probe, filepath.Join(dir, "hls"), 2)
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Create(context.Background(), Request{FileID: 1, Path: src, Duration: 40, Height: 360, BurnSubtitle: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(s.ID)
	const n = 5 // 20s in: not on one of the source's keyframes (every 3s)
	seg, err := m.Segment(context.Background(), s.ID, n)
	if err != nil {
		t.Fatalf("segment %d: %v (%s)", n, err, s.stderr.String())
	}
	out, err := exec.Command(probe, "-v", "error", "-show_entries", "stream=codec_type,start_time", "-of", "csv=p=0", seg).Output()
	if err != nil {
		t.Fatal(err)
	}
	starts := map[string]float64{}
	for _, l := range strings.Fields(string(out)) {
		if f := strings.Split(l, ","); len(f) == 2 {
			starts[f[0]], _ = strconv.ParseFloat(f[1], 64)
		}
	}
	v, ok := starts["video"]
	if !ok {
		t.Fatalf("segment %d has no video (%s)", n, out)
	}
	// The MPEG-TS muxer adds its usual 1.4s; anything later means the run
	// didn't start on the grid.
	if want := float64(n*SegDur) + 1.4; v < want-0.1 || v > want+0.2 {
		t.Fatalf("segment %d's video starts at %.3f, want about %.1f (audio %.3f)", n, v, want, starts["audio"])
	}
}
