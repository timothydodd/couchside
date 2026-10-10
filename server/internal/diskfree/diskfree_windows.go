//go:build windows

package diskfree

import "golang.org/x/sys/windows"

func of(path string) (Space, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Space{}, err
	}
	var free, total, all uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &all); err != nil {
		return Space{}, err
	}
	return Space{Free: free, Total: total}, nil
}
