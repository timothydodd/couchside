package livetv

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/transcode"
	"github.com/timothydodd/couchside/internal/usererr"
)

// loopRig drives virtualLoop on a clock the test owns: sleeping and playing
// in real time both move it forward.
type loopRig struct {
	now    int64
	pieces []db.PlayoutPiece
	broken map[string]bool  // files that fail to play
	short  map[string]int64 // files that end after this many ms
	runs   []loopRun
	cancel context.CancelFunc
	stopAt int64
}

type loopRun struct {
	path       string
	at, in, ms int64
	realtime   bool
}

func (r *loopRig) src(_ context.Context, ms int64) ([]db.PlayoutPiece, error) {
	for i, p := range r.pieces {
		if p.EndMs > ms {
			return r.pieces[i:], nil
		}
	}
	return nil, nil
}

func (r *loopRig) play(_ context.Context, p db.PlayoutPiece, inMs, durMs, _ int64, realtime bool, _ int) (int64, error) {
	r.runs = append(r.runs, loopRun{p.Path, r.now, inMs, durMs, realtime})
	if r.broken[p.Path] {
		r.now += 200 // ffmpeg starting and failing
		return 0, errors.New("No such file or directory")
	}
	got := durMs
	if left, ok := r.short[p.Path]; ok {
		got = max(0, min(durMs, left-inMs))
	}
	if realtime {
		r.now += got
	} else {
		r.now += got / 10
	}
	if r.now >= r.stopAt {
		r.cancel()
	}
	return got, nil
}

func (r *loopRig) sleep(_ context.Context, d time.Duration) {
	r.now += d.Milliseconds()
	if r.now >= r.stopAt {
		r.cancel()
	}
}

func (r *loopRig) run(t *testing.T) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	failed := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		virtualLoop(ctx, "900", r.src, r.play, func() int64 { return r.now }, r.sleep, failed)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the loop didn't finish")
	}
	cancel()
	select {
	case err := <-failed:
		return err
	default:
		return nil
	}
}

// firstRun is the first time a file was played.
func (r *loopRig) firstRun(path string) (loopRun, bool) {
	for _, x := range r.runs {
		if x.path == path {
			return x, true
		}
	}
	return loopRun{}, false
}

func minutes(n int64) int64 { return n * 60_000 }

func threeShows() []db.PlayoutPiece {
	return []db.PlayoutPiece{
		{StartMs: 0, EndMs: minutes(30), Path: "/tv/a.mkv", HasAudio: true},
		{StartMs: minutes(30), EndMs: minutes(60), Path: "/tv/b.mkv", HasAudio: true},
		{StartMs: minutes(60), EndMs: minutes(90), Path: "/tv/c.mkv", HasAudio: true},
	}
}

// A file that won't play leaves the stream paused for its slot; the next
// program starts when the guide says, not straight away.
func TestVirtualLoopStaysOnScheduleAfterAFailure(t *testing.T) {
	r := &loopRig{now: minutes(10), pieces: threeShows(), broken: map[string]bool{"/tv/b.mkv": true}, stopAt: minutes(70)}
	if err := r.run(t); err != nil {
		t.Fatal(err)
	}
	c, ok := r.firstRun("/tv/c.mkv")
	if !ok {
		t.Fatalf("the third program never started: %+v", r.runs)
	}
	if early := minutes(60) - c.at; early > 2000 || early < -virtualMaxLagMs {
		t.Fatalf("the third program started at %dms, scheduled at %dms", c.at, minutes(60))
	}
	if c.in > 2000 {
		t.Fatalf("the third program started %dms in", c.in)
	}
	// The broken file is tried again while it's scheduled, but not in a spin.
	tries := 0
	for _, x := range r.runs {
		if x.path == "/tv/b.mkv" {
			tries++
		}
	}
	if tries < 2 || tries > 70 {
		t.Fatalf("the broken file was tried %d times in its 30 minutes", tries)
	}
}

// A file shorter than its schedule slot: the rest of the slot is a pause.
func TestVirtualLoopShortFile(t *testing.T) {
	r := &loopRig{now: minutes(10), pieces: threeShows(), short: map[string]int64{"/tv/a.mkv": minutes(20)}, stopAt: minutes(40)}
	if err := r.run(t); err != nil {
		t.Fatal(err)
	}
	b, ok := r.firstRun("/tv/b.mkv")
	if !ok {
		t.Fatalf("the second program never started: %+v", r.runs)
	}
	if early := minutes(30) - b.at; early > 2000 || early < -virtualMaxLagMs {
		t.Fatalf("the second program started at %dms, scheduled at %dms", b.at, minutes(30))
	}
}

// Nothing fails: each program follows the last, in real time after the
// fast start.
func TestVirtualLoopNormal(t *testing.T) {
	r := &loopRig{now: minutes(10), pieces: threeShows(), stopAt: minutes(65)}
	if err := r.run(t); err != nil {
		t.Fatal(err)
	}
	if len(r.runs) < 3 || r.runs[0].realtime || r.runs[0].in != minutes(10)-virtualLeadMs {
		t.Fatalf("first run should catch up fast from 8s back: %+v", r.runs)
	}
	for _, path := range []string{"/tv/b.mkv", "/tv/c.mkv"} {
		x, ok := r.firstRun(path)
		if !ok || !x.realtime || x.in != 0 {
			t.Fatalf("%s: %+v (played %v)", path, x, ok)
		}
	}
	if len(r.runs) > 6 {
		t.Fatalf("too many runs for three programs: %+v", r.runs)
	}
}

// Tuning in while a broken file is on: wait for the next program when it's
// seconds away, otherwise say when the channel is back.
func TestVirtualLoopTuneInToABrokenFile(t *testing.T) {
	r := &loopRig{now: minutes(10), pieces: threeShows(), broken: map[string]bool{"/tv/a.mkv": true}, stopAt: minutes(40)}
	err := r.run(t)
	if err == nil || !usererr.Is(err) || !strings.Contains(err.Error(), "a.mkv") || strings.Contains(err.Error(), "/tv/") {
		t.Fatalf("tune-in to a broken file with 20 minutes left = %v", err)
	}

	r = &loopRig{now: minutes(30) - 5000, pieces: threeShows(), broken: map[string]bool{"/tv/a.mkv": true}, stopAt: minutes(35)}
	if err := r.run(t); err != nil {
		t.Fatalf("tune-in 5s before the next program = %v", err)
	}
	b, ok := r.firstRun("/tv/b.mkv")
	if !ok || b.at < minutes(30)-1500 {
		t.Fatalf("the next program: %+v (played %v)", b, ok)
	}
}

// A piece's ffmpeg arguments tone map an HDR file and use the GPU pipeline
// when asked; a plain file gets neither.
func TestVirtualArgsHDRAndGPU(t *testing.T) {
	m := &liveManager{enc: transcode.Encoder{FFmpeg: "ffmpeg", HW: "vaapi", VAAPIDevice: "/dev/dri/renderD128", Tonemap: true, HWDecode: true, HWTonemap: true}}
	p := db.PlayoutPiece{StartMs: 0, EndMs: 60_000, Path: "/films/x.mkv", HasAudio: true}
	spec := Spec{Height: 720}
	args := func(v pieceVideo, hw bool) string {
		return strings.Join(m.virtualArgs(p, v, hw, 0, 60_000, 0, true, spec, "/tmp/s", 1), " ")
	}
	plain := args(pieceVideo{Codec: "h264"}, false)
	if strings.Contains(plain, "tonemap") || strings.Contains(plain, "-hwaccel") {
		t.Fatalf("a plain file on the CPU path: %s", plain)
	}
	hdr := args(pieceVideo{HDR: true, Codec: "hevc"}, true)
	if !strings.Contains(hdr, "tonemap_vaapi") || !strings.Contains(hdr, "-hwaccel vaapi") {
		t.Fatalf("an HDR file on the GPU path: %s", hdr)
	}
	cpu := args(pieceVideo{HDR: true, Codec: "hevc"}, false)
	if !strings.Contains(cpu, "tonemap=tonemap=hable") || strings.Contains(cpu, "-hwaccel") {
		t.Fatalf("an HDR file on the CPU path: %s", cpu)
	}
	// Smaller sources are still scaled to the session's height.
	if !strings.Contains(plain, "scale=-2:720") {
		t.Fatalf("output isn't the session's height: %s", plain)
	}
}
