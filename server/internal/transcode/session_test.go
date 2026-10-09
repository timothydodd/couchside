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
	req.VideoCodecs = []string{" H265 "} // however the client spells it
	if s, err = m.Create(context.Background(), req); err != nil || !s.CopyVideo {
		t.Fatalf("HEVC asked for as H265: copy=%v err=%v", s != nil && s.CopyVideo, err)
	}
	req.Height = 720 // smaller than the source: has to be encoded
	if s, err = m.Create(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if s.CopyVideo {
		t.Fatal("copied a 1080p source for a 720p request")
	}
}

// Dolby Vision profile 5 probes as hevc, but only a Dolby Vision player can
// show it: it's never copied on the strength of "plays hevc".
func TestDolbyVisionProfile5IsNotCopied(t *testing.T) {
	ffmpeg, _, _ := fakeTools(t)
	ffprobe := filepath.Join(t.TempDir(), "ffprobe")
	out := `{"format":{"duration":"600"},"streams":[{"codec_type":"video","codec_name":"hevc","width":3840,"height":2160,"pix_fmt":"yuv420p10le","side_data_list":[{"side_data_type":"DOVI configuration record","dv_profile":5}]},{"codec_type":"audio","codec_name":"eac3","channels":6}]}`
	if err := os.WriteFile(ffprobe, []byte("#!/bin/sh\necho '"+out+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(Encoder{FFmpeg: ffmpeg}, ffprobe, filepath.Join(t.TempDir(), "hls"), 4)
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Create(context.Background(), Request{FileID: 1, Path: "/x.mkv", Duration: 600, AllowCopyVideo: true, VideoCodecs: []string{"hevc"}, BurnSubtitle: -1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close(s.ID) })
	if s.CopyVideo {
		t.Fatal("Dolby Vision profile 5 was copied")
	}
}

// The test file's audio is 5.1 AC-3. A browser gets stereo AAC; a TV that
// plays AC-3 gets it untouched; one that only plays E-AC-3 gets 5.1 E-AC-3.
func TestSurroundAudio(t *testing.T) {
	m, _ := newTestManager(t, 8)
	req := Request{FileID: 1, Path: "/x.mkv", Duration: 600, Height: 1080, AllowCopyAudio: true, BurnSubtitle: -1}
	create := func(codecs ...string) *Session {
		t.Helper()
		req.AudioCodecs = codecs
		s, err := m.Create(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		m.Close(s.ID)
		return s
	}
	if s := create(); s.CopyAudio || s.AudioOut != "aac" {
		t.Fatalf("a browser: copy=%v out=%s", s.CopyAudio, s.AudioOut)
	}
	if s := create("ac3", "eac3"); !s.CopyAudio || s.AudioOut != "copy" {
		t.Fatalf("a TV that plays AC-3: copy=%v out=%s", s.CopyAudio, s.AudioOut)
	}
	if s := create(" AC-3 "); !s.CopyAudio {
		t.Fatalf("AC-3 spelled the client's way wasn't recognised")
	}
	if s := create("eac3"); s.CopyAudio || s.AudioOut != "eac3" {
		t.Fatalf("a TV that only plays E-AC-3: copy=%v out=%s", s.CopyAudio, s.AudioOut)
	}
	req.AllowCopyAudio = false
	if s := create("ac3"); s.CopyAudio || s.AudioOut != "ac3" {
		t.Fatalf("copying not allowed: copy=%v out=%s", s.CopyAudio, s.AudioOut)
	}
	if got := strings.Join(audioArgs("eac3"), " "); got != "-c:a eac3 -ac 6 -b:a 640k" {
		t.Fatalf("eac3 args = %s", got)
	}
	if got := strings.Join(audioArgs("aac"), " "); got != strings.Join(AudioArgs(), " ") {
		t.Fatalf("aac args = %s", got)
	}
}

// Segments more than a minute behind the player go; the rest, the ffmpeg
// playlist and a half-written segment stay.
func TestTrimKeepsAMinuteBehind(t *testing.T) {
	m, _ := newTestManager(t, 2)
	s, err := m.Create(context.Background(), Request{FileID: 1, Path: "/x.mkv", Duration: 600, BurnSubtitle: -1})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 40; i++ {
		os.WriteFile(s.segPath(i), nil, 0o644)
	}
	os.WriteFile(filepath.Join(s.dir, "seg5.ts.tmp"), nil, 0o644)
	os.WriteFile(filepath.Join(s.dir, "ffmpeg.m3u8"), nil, 0o644)
	s.mu.Lock()
	s.lastReq = 30
	s.trimBehind()
	s.mu.Unlock()
	for i := 0; i <= 40; i++ {
		_, err := os.Stat(s.segPath(i))
		if gone := os.IsNotExist(err); gone != (i < 30-keepBehind) {
			t.Errorf("seg%d gone = %v", i, gone)
		}
	}
	for _, n := range []string{"seg5.ts.tmp", "ffmpeg.m3u8"} {
		if _, err := os.Stat(filepath.Join(s.dir, n)); err != nil {
			t.Errorf("%s was removed", n)
		}
	}
}

// A segment the current run wrote and the trim deleted is made again: ffmpeg
// restarts there, instead of the request waiting for a file that never comes.
func TestMissingTrimmedSegmentRestartsFfmpeg(t *testing.T) {
	m, pids := newTestManager(t, 2)
	s, err := m.Create(context.Background(), Request{FileID: 1, Path: "/x.mkv", Duration: 600, BurnSubtitle: -1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	m.Segment(ctx, s.ID, 20) // starts a run at 20; the fake writes nothing
	cancel()
	if started(pids) != 1 {
		t.Fatalf("runs = %d, want 1", started(pids))
	}
	for i := 20; i <= 45; i++ {
		os.WriteFile(s.segPath(i), nil, 0o644)
	}
	s.mu.Lock()
	s.lastReq = 40 // the player is at 40: 20 to 24 are trimmed
	s.trimBehind()
	s.mu.Unlock()
	if _, err := os.Stat(s.segPath(22)); !os.IsNotExist(err) {
		t.Fatal("segment 22 wasn't trimmed")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		// The restarted run "writes" segment 22.
		deadline := time.Now().Add(2 * time.Second)
		for started(pids) < 2 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		os.WriteFile(s.segPath(22), nil, 0o644)
	}()
	if _, err := m.Segment(ctx, s.ID, 22); err != nil {
		t.Fatalf("segment 22: %v (runs %d, hi %d, startSeg %d, running %v)", err, started(pids), s.hi, s.startSeg, s.running())
	}
	if started(pids) != 2 {
		t.Fatalf("runs = %d, want a restart at the trimmed segment", started(pids))
	}
}

// A remux is paused closer to the player than a transcode.
func TestRemuxRunsLessFarAhead(t *testing.T) {
	m, _ := newTestManager(t, 4)
	req := Request{FileID: 1, Path: "/x.mkv", Duration: 600, Height: 1080, AllowCopyVideo: true, BurnSubtitle: -1}
	s, err := m.Create(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if s.CopyVideo || s.aheadLimit() != aheadLimit {
		t.Fatalf("transcode: copy %v, ahead %d", s.CopyVideo, s.aheadLimit())
	}
	req.VideoCodecs = []string{"hevc"}
	if s, err = m.Create(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if !s.CopyVideo || s.aheadLimit() != remuxAhead {
		t.Fatalf("remux: copy %v, ahead %d", s.CopyVideo, s.aheadLimit())
	}
}
