package livetv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timothydodd/couchside/internal/transcode"
)

func TestWatchOptsSpec(t *testing.T) {
	roku := WatchOpts{Height: 720, VideoCodecs: []string{"mpeg2", "mpeg4 avc"}, AudioCodecs: []string{"ac3", "aac"}}
	if sp := roku.spec("MPEG2", "AC3"); !sp.CopyVideo || !sp.CopyAudio || sp.VideoCodec != "mpeg2" {
		t.Errorf("Roku on an MPEG-2/AC-3 channel: %+v, want both passed through", sp)
	}
	if sp := roku.spec("HEVC", "AC4"); sp.CopyVideo || sp.CopyAudio {
		t.Errorf("Roku on ATSC 3.0: %+v, want transcoding", sp)
	}
	if sp := roku.spec("H264", "AC3"); !sp.CopyVideo {
		t.Errorf("Roku on an H.264 channel: %+v", sp)
	}
	browser := WatchOpts{}
	if sp := browser.spec("MPEG2", "AC3"); sp.CopyVideo || sp.CopyAudio || sp.Height != 720 {
		t.Errorf("browser: %+v, want a 720p transcode", sp)
	}
	if sp := roku.spec("", ""); sp.CopyVideo || sp.CopyAudio {
		t.Errorf("unknown codecs must transcode: %+v", sp)
	}
}

// A broadcast-like stream (interlaced MPEG-2 + AC-3 in MPEG-TS) through both
// paths with the real ffmpeg: passed through, and transcoded in software.
func TestLiveStreamsWithFFmpeg(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "broadcast.ts")
	gen := exec.Command(ff, "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=30000/1001", "-f", "lavfi",
		"-i", "sine=frequency=440", "-t", "6", "-c:v", "mpeg2video", "-flags", "+ilme+ildct", "-top", "1", "-g", "15",
		"-b:v", "6M", "-c:a", "ac3", "-ac", "2", "-f", "mpegts", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("making a test broadcast: %v %s", err, out)
	}
	m, err := newLiveManager(transcode.Encoder{FFmpeg: ff, HW: "none"}, filepath.Join(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	probe := func(seg string) string {
		out, _ := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_name,height", "-of", "csv=p=0", seg).Output()
		// MPEG-TS lists each stream twice (once under its program): keep the first of each.
		var seen []string
		for _, f := range strings.Fields(string(out)) {
			if len(seen) == 0 || !strings.Contains(" "+strings.Join(seen, " ")+" ", " "+f+" ") {
				seen = append(seen, f)
			}
		}
		return strings.Join(seen, " ")
	}
	for _, c := range []struct {
		spec Spec
		want string
	}{
		{Spec{Height: 720, CopyVideo: true, CopyAudio: true, VideoCodec: "mpeg2"}, "mpeg2video,1080 ac3"},
		{Spec{Height: 720, VideoCodec: "mpeg2"}, "h264,720 aac"},
	} {
		s, err := m.startInput(context.Background(), "test", "1", "Test", []string{"-re", "-i", src}, c.spec)
		if err != nil {
			t.Fatalf("%+v: %v", c.spec, err)
		}
		<-s.exited
		seg := filepath.Join(s.dir, "seg0.ts")
		if _, err := os.Stat(seg); err != nil {
			t.Fatalf("%+v: no first segment (%s)", c.spec, s.stderr.String())
		}
		if got := probe(seg); got != c.want {
			t.Errorf("%+v: segment is %q, want %q", c.spec, got, c.want)
		}
		if s.CopyVideo != c.spec.CopyVideo || (s.CopyVideo && s.HW != "") {
			t.Errorf("session flags %+v", s)
		}
	}
}
