package logbuf

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestBufferKeepsLinesAndPassesThem(t *testing.T) {
	var out bytes.Buffer
	b := New(8)
	log := slog.New(b.Handler(slog.NewTextHandler(&out, nil))).With("job", 7).WithGroup("scan")
	log.Info("scan complete", "library", "TV Shows", "added", 2)
	log.Debug("not enabled")
	log.Warn("x", slog.Group("file", "id", 3))

	got, last, gap := b.Since(0)
	if len(got) != 2 || last != 2 || gap {
		t.Fatalf("entries %+v last %d gap %v", got, last, gap)
	}
	if e := got[0]; e.Level != "INFO" || e.Msg != "scan complete" || e.Attrs != `job=7 scan.library="TV Shows" scan.added=2` {
		t.Fatalf("first entry %+v", e)
	}
	if got[1].Attrs != "job=7 scan.file.id=3" {
		t.Fatalf("group attrs %q", got[1].Attrs)
	}
	if !strings.Contains(out.String(), "scan complete") {
		t.Fatalf("stdout handler didn't get the line: %q", out.String())
	}
	if more, _, _ := b.Since(last); len(more) != 0 {
		t.Fatalf("nothing new, got %v", more)
	}
}

func TestBufferDropsOldLines(t *testing.T) {
	b := New(8)
	log := slog.New(b.Handler(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	for range 20 {
		log.Info("line")
	}
	all, last, _ := b.Since(0)
	if len(all) > 8 || all[len(all)-1].Seq != 20 || last != 20 {
		t.Fatalf("kept %d, last seq %d", len(all), all[len(all)-1].Seq)
	}
	if _, _, gap := b.Since(2); !gap {
		t.Fatal("lines after 2 were dropped, want gap")
	}
	if got, _, gap := b.Since(18); gap || len(got) != 2 {
		t.Fatalf("after 18: %d lines, gap %v", len(got), gap)
	}
	// A client from before a restart has a seq past the end.
	if got, _, gap := b.Since(500); !gap || len(got) != len(all) {
		t.Fatalf("after a restart: %d lines, gap %v", len(got), gap)
	}
}
