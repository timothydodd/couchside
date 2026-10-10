package transcode

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// A run making video slower than it plays is logged once; a fast one, or one
// just started, isn't.
func TestSlowWarn(t *testing.T) {
	var out bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	fast := &Session{ID: "fast", runStart: time.Now().Add(-20 * time.Second)}
	fast.slowWarn(9) // 40s of video in 20s
	young := &Session{ID: "young", runStart: time.Now().Add(-5 * time.Second)}
	young.slowWarn(0)
	slow := &Session{ID: "slow", runStart: time.Now().Add(-20 * time.Second)}
	slow.slowWarn(1) // 8s of video in 20s
	slow.slowWarn(2)
	if got := strings.Count(out.String(), "slower than real time"); got != 1 || !strings.Contains(out.String(), "session=slow") {
		t.Fatalf("log = %q", out.String())
	}
}
