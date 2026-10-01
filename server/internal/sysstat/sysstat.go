// Package sysstat samples CPU and memory use for the Settings page: the
// container's (cgroup v2) when Couchside runs in one, otherwise the host's,
// plus Couchside's own process and its ffmpeg/comskip children. It reads
// Linux's /proc and /sys files; elsewhere Available is false.
package sysstat

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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
	cgroup  = "/sys/fs/cgroup"
	clkTck  = 100 // USER_HZ: /proc CPU times are in 1/100 s on every Linux we run on
	minGap  = 200 * time.Millisecond
	maxGap  = 10 * time.Second
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
	scope, inContainer := "host", exists(filepath.Join(cgroup, "memory.current"))
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

func take(inContainer bool) (*sample, bool) {
	s := &sample{at: time.Now(), procs: map[int]procSample{}}
	var ok bool
	if inContainer {
		s.busy, ok = cgroupCPU()
	} else {
		s.busy, ok = hostCPU()
	}
	if !ok {
		return nil, false
	}
	self := os.Getpid()
	dirs, _ := os.ReadDir("/proc")
	for _, d := range dirs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		name := readTrim(filepath.Join("/proc", d.Name(), "comm"))
		if pid != self && !encoders[name] {
			continue
		}
		if p, ok := readProc(pid); ok {
			p.name = name
			s.procs[pid] = p
		}
	}
	return s, true
}

// cgroupCPU is the container's CPU seconds used, from cpu.stat.
func cgroupCPU() (float64, bool) {
	b, err := os.ReadFile(filepath.Join(cgroup, "cpu.stat"))
	if err != nil {
		return 0, false
	}
	return ParseCgroupCPU(b)
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

// hostCPU is the host's busy CPU seconds, from /proc/stat.
func hostCPU() (float64, bool) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, false
	}
	return ParseProcStat(b)
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

// cores is the CPU capacity: the container's quota, or every core.
func cores(inContainer bool) float64 {
	if inContainer {
		if q, ok := ParseCPUMax(readTrim(filepath.Join(cgroup, "cpu.max"))); ok {
			return q
		}
	}
	return float64(runtime.NumCPU())
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

// memory is used and total bytes. In a container, used is the working set
// (what Kubernetes counts: memory.current less reclaimable file cache) and
// total is memory.max, or the host's RAM when unlimited.
func memory(inContainer bool) (used, total uint64) {
	hostTotal, hostAvail := meminfo()
	if !inContainer {
		return hostTotal - min(hostAvail, hostTotal), hostTotal
	}
	cur, _ := strconv.ParseUint(readTrim(filepath.Join(cgroup, "memory.current")), 10, 64)
	inactive := statValue(filepath.Join(cgroup, "memory.stat"), "inactive_file")
	used = cur - min(inactive, cur)
	total = hostTotal
	if m, err := strconv.ParseUint(readTrim(filepath.Join(cgroup, "memory.max")), 10, 64); err == nil && (total == 0 || m < total) {
		total = m
	}
	return used, total
}

func meminfo() (total, avail uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), ":")
		kb, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
		switch k {
		case "MemTotal":
			total = kb * 1024
		case "MemAvailable":
			avail = kb * 1024
		}
	}
	return total, avail
}

func statValue(path, key string) uint64 {
	b, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, key+" "); ok {
			n, _ := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
			return n
		}
	}
	return 0
}

// readProc reads a process's CPU seconds (utime+stime) and resident memory.
func readProc(pid int) (procSample, bool) {
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	b, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return procSample{}, false
	}
	cpu, ok := ParsePidStat(b)
	if !ok {
		return procSample{}, false
	}
	return procSample{cpu: cpu, rss: vmRSS(filepath.Join(dir, "status"))}, true
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

func vmRSS(path string) uint64 {
	b, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			kb, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
			return kb * 1024
		}
	}
	return 0
}

func readTrim(path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func clamp(p float64) float64 { return max(0, min(100, p)) }
