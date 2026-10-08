package sysstat

import (
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows: the machine's CPU from GetSystemTimes, memory from
// GlobalMemoryStatusEx, and Couchside's and its encoders' CPU and working
// set from a process snapshot. There are no containers to look inside.

var (
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes = kernel32.NewProc("GetSystemTimes")
	procGetMemoryInfo  = kernel32.NewProc("K32GetProcessMemoryInfo")
	procMemoryStatus   = kernel32.NewProc("GlobalMemoryStatusEx")
)

// memoryStatus is MEMORYSTATUSEX.
type memoryStatus struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// memoryCounters is PROCESS_MEMORY_COUNTERS.
type memoryCounters struct {
	Size                       uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func inContainer() bool { return false }

func take(bool) (*sample, bool) {
	s := &sample{at: time.Now(), procs: map[int]procSample{}}
	var idle, kernel, user windows.Filetime
	if r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); r == 0 {
		return nil, false
	}
	// Kernel time includes idle time.
	s.busy = seconds(kernel) + seconds(user) - seconds(idle)

	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return s, true // the totals still stand
	}
	defer windows.CloseHandle(snap)
	self := uint32(windows.GetCurrentProcessId())
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := strings.TrimSuffix(strings.ToLower(windows.UTF16ToString(e.ExeFile[:])), ".exe")
		if e.ProcessID != self && !encoders[name] {
			continue
		}
		if p, ok := readProc(e.ProcessID); ok {
			p.name = name
			s.procs[int(e.ProcessID)] = p
		}
	}
	return s, true
}

// readProc is a process's CPU seconds (kernel+user) and working set.
func readProc(pid uint32) (procSample, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return procSample{}, false
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return procSample{}, false
	}
	p := procSample{cpu: seconds(kernel) + seconds(user)}
	mem := memoryCounters{Size: uint32(unsafe.Sizeof(memoryCounters{}))}
	if r, _, _ := procGetMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mem)), uintptr(mem.Size)); r != 0 {
		p.rss = uint64(mem.WorkingSetSize)
	}
	return p, true
}

func cores(bool) float64 { return float64(runtime.NumCPU()) }

func memory(bool) (used, total uint64) {
	m := memoryStatus{Length: uint32(unsafe.Sizeof(memoryStatus{}))}
	if r, _, _ := procMemoryStatus.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0, 0
	}
	return m.TotalPhys - min(m.AvailPhys, m.TotalPhys), m.TotalPhys
}

// seconds is a FILETIME duration (100 ns ticks) in seconds.
func seconds(f windows.Filetime) float64 {
	return float64(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) / 1e7
}
