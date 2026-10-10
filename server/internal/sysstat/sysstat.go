// Package sysstat samples CPU and memory use for the Settings page: the
// container's (cgroup v2) when Couchside runs in one, otherwise the host's,
// plus Couchside's own process and its ffmpeg/comskip children. Linux reads
// /proc and /sys (read_unix.go), Windows asks the system (read_windows.go);
// elsewhere Available is false.
package sysstat

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Stats is one reading. CPU percentages are of the whole capacity (Cores),
// so 100 means every core is busy.
type Stats struct {
	Available  bool    `json:"available"`
	Scope      string  `json:"scope"` // container | host
	Cores      float64 `json:"cores"`
	CPUPercent float64 `json:"cpuPercent"`
	MemUsed    uint64  `json:"memUsed"`
	MemTotal   uint64  `json:"memTotal"`
	Server     Procs   `json:"server"`   // the Couchside process
	Encoders   Procs   `json:"encoders"` // ffmpeg and comskip processes
}

// Procs is a group of processes.
type Procs struct {
	Count      int     `json:"count"`
	CPUPercent float64 `json:"cpuPercent"`
	RSS        uint64  `json:"rss"`
}

const (
	cgroup = "/sys/fs/cgroup"
	clkTck = 100 // USER_HZ: /proc CPU times are in 1/100 s on every Linux we run on
	minGap = 200 * time.Millisecond
	maxGap = 10 * time.Second
)

// encoders are the child processes counted as transcoding work.
var encoders = map[string]bool{"ffmpeg": true, "comskip": true}

// sample is the raw counters at one moment.
type sample struct {
	at    time.Time
	busy  float64            // CPU seconds used by the scope
	procs map[int]procSample // per pid
}

type procSample struct {
	name string
	cpu  float64 // CPU seconds
	rss  uint64
}

// Sampler turns successive samples into rates.
type Sampler struct {
	mu   sync.Mutex
	last *sample
}

// Read returns current use. CPU is averaged since the previous call; when
// that was too long ago (or never) it measures over a short pause instead.
func (s *Sampler) Read() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, inContainer := "host", inContainer()
	if inContainer {
		scope = "container"
	}
	cur, ok := take(inContainer)
	if !ok {
		return Stats{}
	}
	prev := s.last
	if prev == nil || cur.at.Sub(prev.at) > maxGap || cur.at.Sub(prev.at) < minGap {
		prev = cur
		time.Sleep(minGap + 50*time.Millisecond)
		if cur, ok = take(inContainer); !ok {
			return Stats{}
		}
	}
	s.last = cur

	st := Stats{Available: true, Scope: scope, Cores: cores(inContainer)}
	wall := cur.at.Sub(prev.at).Seconds()
	capacity := wall * st.Cores
	pct := func(cpu float64) float64 { return clamp(cpu / capacity * 100) }
	st.CPUPercent = pct(cur.busy - prev.busy)
	st.MemUsed, st.MemTotal = memory(inContainer)

	self := os.Getpid()
	for pid, p := range cur.procs {
		var group *Procs
		switch {
		case pid == self:
			group = &st.Server
		case encoders[p.name]:
			group = &st.Encoders
		default:
			continue
		}
		group.Count++
		group.RSS += p.rss
		if old, ok := prev.procs[pid]; ok && old.cpu <= p.cpu {
			group.CPUPercent += pct(p.cpu - old.cpu)
		}
	}
	st.Encoders.CPUPercent = clamp(st.Encoders.CPUPercent)
	return st
}

// ParseCgroupCPU reads usage_usec from a cgroup v2 cpu.stat file, in seconds.
func ParseCgroupCPU(b []byte) (float64, bool) {
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "usage_usec "); ok {
			n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			return n / 1e6, err == nil
		}
	}
	return 0, false
}

// ParseProcStat sums the busy jiffies (everything but idle and iowait) on
// /proc/stat's "cpu" line, in seconds.
func ParseProcStat(b []byte) (float64, bool) {
	line, _, _ := bytes.Cut(b, []byte("\n"))
	f := strings.Fields(string(line))
	if len(f) < 5 || f[0] != "cpu" {
		return 0, false
	}
	var busy float64
	for i, v := range f[1:] {
		if i == 3 || i == 4 || i >= 8 { // idle, iowait; guest time is already in user
			continue
		}
		n, _ := strconv.ParseFloat(v, 64)
		busy += n
	}
	return busy / clkTck, true
}

// ParseCPUMax turns cgroup v2 cpu.max ("200000 100000") into cores; "max" means no limit.
func ParseCPUMax(s string) (float64, bool) {
	f := strings.Fields(s)
	if len(f) != 2 || f[0] == "max" {
		return 0, false
	}
	quota, err1 := strconv.ParseFloat(f[0], 64)
	period, err2 := strconv.ParseFloat(f[1], 64)
	if err1 != nil || err2 != nil || period <= 0 || quota <= 0 {
		return 0, false
	}
	return quota / period, true
}

// ParsePidStat returns utime+stime in seconds from /proc/<pid>/stat. The
// command name can hold spaces and parentheses, so fields count from the last ')'.
func ParsePidStat(b []byte) (float64, bool) {
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 13 {
		return 0, false
	}
	ut, err1 := strconv.ParseFloat(f[11], 64)
	st, err2 := strconv.ParseFloat(f[12], 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return (ut + st) / clkTck, true
}

func clamp(p float64) float64 { return max(0, min(100, p)) }

// InContainer says whether the server runs in a container (cgroup v2 scope).
func InContainer() bool { return inContainer() }
