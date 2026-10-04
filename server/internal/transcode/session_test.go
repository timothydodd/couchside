//go:build !windows

package transcode

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fakeTools writes an ffprobe that describes a 10-minute HEVC file and an
// ffmpeg that waits to be killed, never writing a segment. Each run's pid
// goes to the pids file.
func fakeTools(t *testing.T) (ffmpeg, ffprobe, pids string) {
	t.Helper()
	dir := t.TempDir()
	pids = filepath.Join(dir, "pids")
	ffprobe = filepath.Join(dir, "ffprobe")
	ffmpeg = filepath.Join(dir, "ffmpeg")
	probeOut := `{"format":{"duration":"600"},"streams":[{"codec_type":"video","codec_name":"hevc","width":1920,"height":1080,"pix_fmt":"yuv420p10le"},{"codec_type":"audio","codec_name":"ac3","channels":6}]}`
	write := func(p, body string) {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(ffprobe, "echo '"+probeOut+"'\n")
	write(ffmpeg, "exec sleep 60\n")
	var mu sync.Mutex
	startHook = func(pid int) {
		mu.Lock()
		defer mu.Unlock()
		f, _ := os.OpenFile(pids, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		f.WriteString(strconv.Itoa(pid) + "\n")
		f.Close()
	}
	t.Cleanup(func() { startHook = nil })
	return ffmpeg, ffprobe, pids
}

// alive counts the fake ffmpegs still running.
func alive(t *testing.T, pids string) int {
	t.Helper()
	b, _ := os.ReadFile(pids)
	n := 0
	for _, l := range strings.Fields(string(b)) {
		pid, _ := strconv.Atoi(l)
		if pid > 0 && syscall.Kill(pid, 0) == nil {
			n++
		}
	}
	return n
}

func started(pids string) int {
	b, _ := os.ReadFile(pids)
	return len(strings.Fields(string(b)))
}

func newTestManager(t *testing.T, max int) (*Manager, string) {
	t.Helper()
	ffmpeg, ffprobe, pids := fakeTools(t)
	m, err := NewManager(Encoder{FFmpeg: ffmpeg}, ffprobe, filepath.Join(t.TempDir(), "hls"), max)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, s := range m.Sessions() {
			m.Close(s.ID)
		}
	})
	return m, pids
}

func TestCreateRespectsTheLimitUnderConcurrency(t *testing.T) {
	m, _ := newTestManager(t, 2)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, busy := 0, 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Create(context.Background(), Request{FileID: 1, Path: "/x.mkv", Duration: 600, BurnSubtitle: -1})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrBusy):
				busy++
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	m.mu.Lock()
	n := len(m.sessions)
	m.mu.Unlock()
	if ok != 2 || busy != 8 || n != 2 {
		t.Fatalf("created %d, busy %d, sessions %d; want 2, 8, 2", ok, busy, n)
	}
}

func TestConcurrentRestartsLeaveOneProcess(t *testing.T) {
	m, pids := newTestManager(t, 2)
	s, err := m.Create(context.Background(), Request{FileID: 1, Path: "/x.mkv", Duration: 600, BurnSubtitle: -1})
	if err != nil {
		t.Fatal(err)
	}
	// Requests far apart each want ffmpeg restarted at their own segment.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
			defer cancel()
			if _, err := m.Segment(ctx, s.ID, n*20); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				t.Log(n, err)
			}
		}(i)
	}
	wg.Wait()
	if started(pids) < 2 {
		t.Fatalf("only %d runs started; the test didn't exercise restarts", started(pids))
	}
	if n := alive(t, pids); n != 1 {
		t.Fatalf("%d ffmpeg processes alive after concurrent restarts (of %d started), want 1", n, started(pids))
	}
	m.Close(s.ID)
	deadline := time.Now().Add(2 * time.Second)
	for alive(t, pids) != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := alive(t, pids); n != 0 {
		t.Fatalf("%d ffmpeg processes alive after Close", n)
	}
}

func TestRestartAfterIdleRespectsTheLimit(t *testing.T) {
	m, _ := newTestManager(t, 1)
	old, err := m.Create(context.Background(), Request{FileID: 1, Path: "/x.mkv", Duration: 600, BurnSubtitle: -1})
	if err != nil {
		t.Fatal(err)
	}
	// It goes quiet long enough for its ffmpeg to be reaped and its slot to free up.
	old.mu.Lock()
	old.lastAccess = time.Now().Add(-2 * idleKill)
	old.mu.Unlock()
	fresh, err := m.Create(context.Background(), Request{FileID: 2, Path: "/y.mkv", Duration: 600, BurnSubtitle: -1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := m.Segment(ctx, fresh.ID, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fresh session segment: %v", err)
	}
	// The old one comes back while the fresh one is busy: no room.
	if _, err := m.Segment(context.Background(), old.ID, 0); !errors.Is(err, ErrBusy) && !errors.Is(err, ErrNoSession) {
		t.Fatalf("reviving past the limit: %v, want ErrBusy", err)
	}
}

// A request waiting on a segment sees the run that a seek just killed. That
// isn't a GPU failure: the session keeps GPU decoding and the seek's run.
func TestSeekIsNotAGPUFailure(t *testing.T) {
	ffmpeg, ffprobe, _ := fakeTools(t)
	m, err := NewManager(Encoder{FFmpeg: ffmpeg, HW: "vaapi", VAAPIDevice: "/dev/dri/renderD128", HWDecode: true}, ffprobe, filepath.Join(t.TempDir(), "hls"), 4)
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Create(context.Background(), Request{FileID: 1, Path: "/x.mkv", Duration: 600, Height: 720, BurnSubtitle: -1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close(s.ID) })
	if !s.HWDecode {
		t.Fatal("the session should start on the GPU pipeline")
	}
	restart := func(n int) *exec.Cmd {
		t.Helper()
		s.restartMu.Lock()
		defer s.restartMu.Unlock()
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.start(n); err != nil {
			t.Fatal(err)
		}
		return s.cmd
	}
	old := restart(0)
	seek := restart(50) // kills the first run

	if !s.gpuFallback(3, "killed", old) {
		t.Fatal("a waiter on the old run should keep waiting")
	}
	s.mu.Lock()
	hw, cur, at := s.HWDecode, s.cmd, s.startSeg
	s.mu.Unlock()
	if !hw || cur != seek || at != 50 {
		t.Fatalf("after a seek: gpu=%v, run replaced=%v, start segment %d", hw, cur != seek, at)
	}

	// The current run dying on its own is a failure: fall back and restart.
	_ = seek.Process.Kill()
	s.mu.Lock()
	exited := s.exited
	s.mu.Unlock()
	<-exited
	if !s.gpuFallback(50, "Failed to create decode context", seek) {
		t.Fatal("a failed GPU run should fall back to CPU decoding")
	}
	s.mu.Lock()
	hw, cur = s.HWDecode, s.cmd
	s.mu.Unlock()
	if hw || cur == seek || cur == nil {
		t.Fatalf("after a real failure: gpu=%v, restarted=%v", hw, cur != seek && cur != nil)
	}
}

func TestVideoCodecsCopyTenBitHEVC(t *testing.T) {
	m, _ := newTestManager(t, 4)
	req := Request{FileID: 1, Path: "/x.mkv", Duration: 600, Height: 1080, AllowCopyVideo: true, BurnSubtitle: -1}
	s, err := m.Create(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if s.CopyVideo {
		t.Fatal("10-bit HEVC copied for a client that only asked for H.264")
	}
	req.VideoCodecs = []string{"hevc"}
	if s, err = m.Create(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if !s.CopyVideo || s.Mode != "remux" || s.HDR {
		t.Fatalf("HEVC not copied for a client that plays it: %+v", s)
	}
	req.Height = 720 // smaller than the source: has to be encoded
	if s, err = m.Create(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if s.CopyVideo {
		t.Fatal("copied a 1080p source for a 720p request")
	}
}
