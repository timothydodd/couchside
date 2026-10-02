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

	"github.com/timothydodd/couchside/internal/transcode"
)

func TestSnapHeight(t *testing.T) {
	for in, want := range map[int]int{0: 720, -5: 720, 1: 360, 359: 360, 360: 360, 500: 480, 719: 480, 720: 720, 1079: 720, 1080: 1080, 4320: 1080} {
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
