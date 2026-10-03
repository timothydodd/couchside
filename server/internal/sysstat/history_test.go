package sysstat

import (
	"testing"
	"time"
)

func TestHistoryWindows(t *testing.T) {
	h := NewHistory(nil)
	start := time.Unix(1_800_000_000, 0) // on a minute boundary
	st := Stats{Available: true, Cores: 4, MemTotal: 1000, Scope: "container"}
	// Two hours of 5s points: CPU 10 in the first minute of each pair, 30 in the second.
	for i := 0; i < 2*3600/5; i++ {
		at := start.Add(time.Duration(i) * Every)
		cpu := 10.0
		if at.Unix()/60%2 == 1 {
			cpu = 30
		}
		h.add(Point{T: at.Unix(), CPU: cpu, Mem: 500, Streams: i % 3}, st)
	}
	now := start.Add(2 * time.Hour)

	fine := h.Points(15*time.Minute, now)
	if fine.Every != 5 || len(fine.Points) != 15*60/5 || fine.Cores != 4 || fine.MemTotal != 1000 {
		t.Fatalf("15m: every %d, %d points, cores %v", fine.Every, len(fine.Points), fine.Cores)
	}
	if all := h.Points(time.Hour, now); len(all.Points) != 3600/5 {
		t.Fatalf("1h kept %d points, want %d", len(all.Points), 3600/5)
	}

	day := h.Points(24*time.Hour, now)
	if day.Every != 60 || len(day.Points) != 120 {
		t.Fatalf("24h: every %d, %d points, want 120 minutes", day.Every, len(day.Points))
	}
	if p := day.Points[1]; p.CPU != 30 || p.Mem != 500 || p.Streams != 2 || p.T != start.Unix()+60 {
		t.Fatalf("second minute averaged to %+v", p)
	}
	if day.Points[0].CPU != 10 {
		t.Fatalf("first minute averaged to %v", day.Points[0].CPU)
	}
}
