//go:build !windows

package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timothydodd/couchside/internal/config"
	"github.com/timothydodd/couchside/internal/db"
	"github.com/timothydodd/couchside/internal/probe"
)

// probeWorker is a worker on a temp DB with one movie library whose ffprobe
// is a script the test rewrites.
func probeWorker(t *testing.T) (w *Worker, d *db.DB, lib int64, root, ffprobe string) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	root = filepath.Join(dir, "media")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if lib, err = d.CreateLibrary(context.Background(), "Films", root, "movies"); err != nil {
		t.Fatal(err)
	}
	ffprobe = filepath.Join(dir, "ffprobe")
	w = &Worker{db: d, cfg: config.Config{FFprobe: ffprobe}, wake: make(chan struct{}, 1), wakeEnc: make(chan struct{}, 1)}
	return w, d, lib, root, ffprobe
}

func setProbe(t *testing.T, path, body string) {
	t.Helper()
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

const goodProbe = `echo '{"format":{"duration":"60"},"streams":[{"codec_type":"video","codec_name":"h264","width":1280,"height":720,"pix_fmt":"yuv420p"}]}'` + "\n"

func TestProbeTimeoutIsRetriedNextScan(t *testing.T) {
	old := probe.Timeout
	probe.Timeout = 300 * time.Millisecond
	t.Cleanup(func() { probe.Timeout = old })
	w, d, lib, root, ffprobe := probeWorker(t)
	ctx := context.Background()
	film := filepath.Join(root, "Heat (1995).mkv")
	if err := os.WriteFile(film, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	setProbe(t, ffprobe, "exec sleep 5\n") // a share too slow to answer
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.FileStamp(ctx, film); st != nil {
		t.Fatalf("a timed-out probe was stored (problem %q)", st.Problem)
	}

	setProbe(t, ffprobe, goodProbe)
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.FileStamp(ctx, film); st == nil || st.Problem != "" {
		t.Fatalf("next scan: %+v, want indexed without a problem", st)
	}
}

func TestUnreadableFileIsProbedAgain(t *testing.T) {
	w, d, lib, root, ffprobe := probeWorker(t)
	ctx := context.Background()
	film := filepath.Join(root, "Heat (1995).mkv")
	if err := os.WriteFile(film, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	setProbe(t, ffprobe, "echo 'Invalid data found' >&2\nexit 1\n")
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.FileStamp(ctx, film); st == nil || st.Problem != "unreadable" {
		t.Fatalf("first scan: %+v, want unreadable", st)
	}
	setProbe(t, ffprobe, goodProbe)
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.FileStamp(ctx, film); st == nil || st.Problem != "" {
		t.Fatalf("rescan of an unchanged unreadable file: %+v, want it probed again and fine", st)
	}
}

// Unchanged files are marked seen in batches (more than one here): none is
// probed again or pruned, and files deleted from disk still are.
func TestScanTouchesUnchangedFilesInBatches(t *testing.T) {
	w, d, lib, root, ffprobe := probeWorker(t)
	ctx := context.Background()
	calls := filepath.Join(filepath.Dir(ffprobe), "calls")
	setProbe(t, ffprobe, "echo x >> '"+calls+"'\n"+goodProbe)
	const n = 520
	var paths []string
	for i := 0; i < n; i++ {
		p := filepath.Join(root, fmt.Sprintf("Film %03d (2001).mkv", i))
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	ids := indexed(t, d, paths...)
	backdate(t, d, ids...)
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	if got := indexed(t, d, paths...); slices.Contains(got, 0) {
		t.Fatal("an unchanged file was pruned: its batch wasn't marked seen")
	}
	b, _ := os.ReadFile(calls)
	if probes := strings.Count(string(b), "x"); probes != n {
		t.Fatalf("ffprobe ran %d times, want %d (unchanged files probed again)", probes, n)
	}
	backdate(t, d, ids...)
	for _, p := range paths[:10] {
		os.Remove(p)
	}
	if err := w.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	got := indexed(t, d, paths...)
	for i, id := range got {
		if (i < 10) != (id == 0) {
			t.Fatalf("file %d: id %d after deleting the first 10", i, id)
		}
	}
}
