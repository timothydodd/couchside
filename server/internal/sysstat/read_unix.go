//go:build !windows

package sysstat

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// inContainer: a cgroup v2 memory controller says Couchside runs in a container.
func inContainer() bool { return exists(filepath.Join(cgroup, "memory.current")) }

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

// hostCPU is the host's busy CPU seconds, from /proc/stat.
func hostCPU() (float64, bool) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, false
	}
	return ParseProcStat(b)
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
