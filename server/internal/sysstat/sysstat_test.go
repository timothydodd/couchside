package sysstat

import "testing"

func TestParsers(t *testing.T) {
	if v, ok := ParseCgroupCPU([]byte("usage_usec 2500000\nuser_usec 2000000\n")); !ok || v != 2.5 {
		t.Errorf("ParseCgroupCPU = %v %v", v, ok)
	}
	// user nice system idle iowait irq softirq steal guest guest_nice
	if v, ok := ParseProcStat([]byte("cpu  100 0 50 900 30 5 5 10 7 0\ncpu0 1 2 3\n")); !ok || v != 1.7 {
		t.Errorf("ParseProcStat = %v %v", v, ok)
	}
	if v, ok := ParseCPUMax("200000 100000"); !ok || v != 2 {
		t.Errorf("ParseCPUMax = %v %v", v, ok)
	}
	if _, ok := ParseCPUMax("max 100000"); ok {
		t.Error("ParseCPUMax(max) should mean no limit")
	}
	stat := "4242 (ffmpeg (x) y) S 1 1 1 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 1 0 100 1000 10"
	if v, ok := ParsePidStat([]byte(stat)); !ok || v != 3 {
		t.Errorf("ParsePidStat = %v %v", v, ok)
	}
}

func TestRead(t *testing.T) {
	var s Sampler
	st := s.Read()
	if st.Available && (st.Cores <= 0 || st.MemTotal == 0 || st.Server.Count != 1) {
		t.Errorf("implausible reading: %+v", st)
	}
}
