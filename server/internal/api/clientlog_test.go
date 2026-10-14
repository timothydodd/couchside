package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
)

// A signed-in client's report lands in the server log on one line, with its
// details flattened and its device named; signed out, it's refused.
func TestClientLog(t *testing.T) {
	var out bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, config.Config{DataDir: dir}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	report := map[string]any{
		"level":   "error",
		"event":   "playback_error",
		"message": "Playback failed:\nunsupported",
		"details": map[string]any{
			"mode": "direct", "fileId": 1807, "height": 2076,
			"errorInfo": map[string]any{"category": "mediaerror", "source": "buffer:loop"},
		},
	}
	if c := newClient(t, ts.URL).do("POST", "/api/client/log", report, nil); c != 401 {
		t.Fatalf("signed out = %d, want 401", c)
	}
	tv := newClient(t, ts.URL)
	tv.header = map[string]string{"User-Agent": "Roku/Couchside-0.10.40"}
	if c := tv.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim"}, nil); c != 200 {
		t.Fatalf("welcome = %d", c)
	}
	if c := tv.do("POST", "/api/client/log", report, nil); c != 204 {
		t.Fatalf("report = %d, want 204", c)
	}
	var line string
	for _, l := range strings.Split(out.String(), "\n") {
		if strings.Contains(l, `msg="client report"`) {
			line = l
		}
	}
	for _, want := range []string{"level=ERROR", "event=playback_error", `message="Playback failed: unsupported"`,
		"device=Roku", "fileId=1807", "height=2076", `errorInfo="category=mediaerror source=buffer:loop"`, "mode=direct"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line %q lacks %q", line, want)
		}
	}
}

// Playback-test reports are also kept in the data folder, one JSON object a
// line, so a run outlives the pod's log; other reports aren't.
func TestPlaytestReportsKept(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(d, config.Config{DataDir: dir}, nil, nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	tv := newClient(t, ts.URL)
	if c := tv.do("POST", "/api/auth/welcome", map[string]string{"name": "Tim"}, nil); c != 200 {
		t.Fatalf("welcome = %d", c)
	}
	for _, ev := range []string{"playtest_result", "playback_started"} {
		r := map[string]any{"level": "warn", "event": ev, "message": "ok clip", "details": map[string]any{"runId": "r1", "n": 3}}
		if c := tv.do("POST", "/api/client/log", r, nil); c != 204 {
			t.Fatalf("%s = %d", ev, c)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "playtest.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("kept %d lines, want only the playtest one: %s", len(lines), b)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["event"] != "playtest_result" || rec["runId"] != "r1" || rec["n"] != "3" || rec["level"] != "WARN" {
		t.Fatalf("kept %v", rec)
	}
}

// A client sending reports in a loop is cut off for the rest of the minute.
func TestReportLimiter(t *testing.T) {
	var l reportLimiter
	now := time.Now()
	for i := range reportPerMin {
		if !l.allow("a", now) {
			t.Fatalf("report %d refused", i)
		}
	}
	if l.allow("a", now) {
		t.Fatal("report past the limit allowed")
	}
	if !l.allow("b", now) {
		t.Fatal("another client refused")
	}
	if !l.allow("a", now.Add(time.Minute)) {
		t.Fatal("refused a minute later")
	}
}
