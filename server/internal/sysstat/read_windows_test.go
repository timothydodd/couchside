package sysstat

import (
	"testing"
	"time"
)

// On a real Windows machine (CI's), the readings are there and plausible.
func TestReadWindows(t *testing.T) {
	var s Sampler
	s.Read()
	for end := time.Now().Add(300 * time.Millisecond); time.Now().Before(end); {
		// keep a core busy so Couchside's own CPU shows
	}
	st := s.Read()
	if !st.Available || st.Scope != "host" || st.Cores < 1 {
		t.Fatalf("stats = %+v", st)
	}
	if st.MemTotal == 0 || st.MemUsed == 0 || st.MemUsed > st.MemTotal {
		t.Fatalf("memory = %d of %d", st.MemUsed, st.MemTotal)
	}
	if st.Server.Count != 1 || st.Server.RSS == 0 || st.Server.CPUPercent <= 0 {
		t.Fatalf("this process = %+v", st.Server)
	}
	if st.CPUPercent < 0 || st.CPUPercent > 100 {
		t.Fatalf("cpu = %v", st.CPUPercent)
	}
}
