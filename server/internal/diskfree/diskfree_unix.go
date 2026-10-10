//go:build !windows

package diskfree

import "syscall"

func of(path string) (Space, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Space{}, err
	}
	bs := uint64(st.Bsize)
	return Space{Free: uint64(st.Bavail) * bs, Total: uint64(st.Blocks) * bs}, nil
}
