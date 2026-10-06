//go:build !windows

package livetv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/transcode"
)

// 0 (and below) is "as broadcast": no scaling at all.
func TestSnapHeight(t *testing.T) {
	for in, want := range map[int]int{0: 0, -5: 0, 1: 360, 359: 360, 360: 360, 500: 480, 719: 480, 720: 720, 1079: 720, 1080: 1080, 4320: 1080} {
		if got := snapHeight(in); got != want {
			t.Errorf("snapHeight(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestLiveArgsWindow(t *testing.T) {
	m := &liveManager{enc: transcode.Encoder{FFmpeg: "ffmpeg", HW: "none"}}
	tuner := strings.Join(m.liveArgs([]string{"-i", "x"}, Spec{Height: 720, window: liveWindow}, false, "/d"), " ")
	if !strings.Contains(tuner, "-hls_list_size 5400") || !strings.Contains(tuner, "delete_segments") || strings.Contains(tuner, "event") {
		t.Errorf("tuner stream args keep every segment: %s", tuner)
	}
	rec := strings.Join(m.liveArgs([]string{"-i", "x"}, Spec{Height: 720}, false, "/d"), " ")
	if !strings.Contains(rec, "-hls_list_size 0") || !strings.Contains(rec, "-hls_playlist_type event") || strings.Contains(rec, "delete_segments") {
		t.Errorf("recording playback must keep its start: %s", rec)
	}
}

// fakeLiveFFmpeg writes its playlist (the last argument) shortly after
// starting, then runs until killed. Each start appends a line to the log.
func fakeLiveFFmpeg(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "starts")
	ff := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho start >> '" + log + "'\nfor last; do :; done\nsleep 0.3\necho '#EXTM3U' > \"$last\"\nexec sleep 60\n"
	if err := os.WriteFile(ff, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return ff, log
}

func starts(log string) int {
	b, _ := os.ReadFile(log)
	return strings.Count(string(b), "start")
}

func TestLiveStartsOncePerStreamAndCapsEncodes(t *testing.T) {
	ff, log := fakeLiveFFmpeg(t)
	m, err := newLiveManager(transcode.Encoder{FFmpeg: ff, HW: "none"}, filepath.Join(t.TempDir(), "live"), 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, s := range m.sessionsList() {
			m.stop(s)
		}
	})
	ctx := context.Background()
	input := []string{"-i", "http://tuner/auto/v2.1"}

	// Ten viewers tune one channel at once, asking for heights 1 to 10: one
	// ffmpeg, one session (every height snaps to 360).
	var wg sync.WaitGroup
	got := make([]*LiveSession, 10)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, err := m.startInput(ctx, "ch:2.1", "2.1", "WSB", input, Spec{Height: i + 1})
			if err != nil {
				t.Error(err)
			}
			got[i] = s
		}(i)
	}
	wg.Wait()
	if n := starts(log); n != 1 {
		t.Fatalf("%d ffmpegs started for one channel, want 1", n)
	}
	for _, s := range got {
		if s == nil || s != got[0] {
			t.Fatal("viewers got different sessions")
		}
	}

	// The encode limit (1) is reached: another channel that needs converting waits…
	if _, err := m.startInput(ctx, "ch:5.1", "5.1", "WAGA", input, Spec{Height: 720}); !errors.Is(err, ErrBusyEncoding) {
		t.Fatalf("second encode: %v, want ErrBusyEncoding", err)
	}
	// …but passing a broadcast through encodes nothing.
	if _, err := m.startInput(ctx, "ch:5.1", "5.1", "WAGA", input, Spec{CopyVideo: true, CopyAudio: true}); err != nil {
		t.Fatalf("passthrough at the encode limit: %v", err)
	}
}

// A recording watched from its start keeps its segments after ffmpeg ends
// (the recording finished), until the viewer stops asking; a tuner stream
// whose ffmpeg died goes at once.
func TestReapKeepsAFinishedRecordingPlayback(t *testing.T) {
	m, err := newLiveManager(transcode.Encoder{FFmpeg: "ffmpeg", HW: "none"}, filepath.Join(t.TempDir(), "live"), 2)
	if err != nil {
		t.Fatal(err)
	}
	ended := func(id, key string) *LiveSession {
		dir := filepath.Join(m.root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		s := &LiveSession{ID: id, key: key, Channel: "2.1", dir: dir, exited: make(chan struct{}), stderr: &syncBuffer{}, lastAccess: time.Now()}
		close(s.exited)
		m.sessions[id] = s
		return s
	}
	rec := ended("r", "rec:7")
	tuner := ended("t", "ch:2.1")
	m.reap()
	if m.get("t") != nil {
		t.Fatal("a tuner stream whose ffmpeg exited was kept")
	}
	if _, err := os.Stat(tuner.dir); err == nil {
		t.Fatal("the dead tuner stream's folder is still there")
	}
	if m.get("r") == nil {
		t.Fatal("a recording's playback was removed while someone was watching it")
	}
	if _, err := os.Stat(rec.dir); err != nil {
		t.Fatal("its segments are gone")
	}
	// The viewer leaves: it goes at the next reap after the idle time.
	rec.mu.Lock()
	rec.lastAccess = time.Now().Add(-liveIdleKill - time.Second)
	rec.mu.Unlock()
	m.reap()
	if m.get("r") != nil {
		t.Fatal("an abandoned recording playback was kept")
	}
}

// The first caller of a shared start leaves before it finishes: someone who
// joined it starts the stream themselves instead of getting an error.
func TestClaimAfterTheFirstCallerLeaves(t *testing.T) {
	m, err := newLiveManager(transcode.Encoder{FFmpeg: "ffmpeg", HW: "none"}, filepath.Join(t.TempDir(), "live"), 2)
	if err != nil {
		t.Fatal(err)
	}
	match := func(*LiveSession) bool { return false }
	m.mu.Lock()
	_, first, err := m.claimLocked(context.Background(), "k", match, true)
	m.mu.Unlock()
	if err != nil || first == nil {
		t.Fatalf("first claim: %v %v", first, err)
	}
	type result struct {
		s   *LiveSession
		f   *liveStart
		err error
	}
	joined := make(chan result, 1)
	go func() {
		m.mu.Lock()
		s, f, err := m.claimLocked(context.Background(), "k", match, true)
		m.mu.Unlock()
		joined <- result{s, f, err}
	}()
	time.Sleep(50 * time.Millisecond)
	m.finish("k", first, nil, context.Canceled)
	select {
	case r := <-joined:
		if r.err != nil || r.s != nil || r.f == nil {
			t.Fatalf("the second caller should start the stream itself: %+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the second caller never got an answer")
	}
}

// launch reports a caller that left as that, not as a weak signal.
func TestLaunchReportsACancelledCaller(t *testing.T) {
	ff := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(ff, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := newLiveManager(transcode.Encoder{FFmpeg: ff, HW: "none"}, filepath.Join(t.TempDir(), "live"), 2)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	_, err = m.launch(ctx, "ch:2.1", "2.1", "WSB", []string{"-i", "x"}, Spec{Height: 720}, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("launch after the caller left = %v, want context.Canceled", err)
	}
}
