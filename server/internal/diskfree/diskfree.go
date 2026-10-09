// Package diskfree reports how much room a folder's disk has.
package diskfree

import (
	"fmt"
	"os"
)

// Space is a disk's free and total bytes, as the server's user sees them.
type Space struct {
	Free  uint64 `json:"free"`
	Total uint64 `json:"total"`
}

// Of returns the space on the disk holding path.
func Of(path string) (Space, error) { return of(path) }

// Human writes a byte count for people: "1.4 GB".
func Human(n uint64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	d, e := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		d *= unit
		e++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(d), "kMGTPE"[e])
}

// FileSizes adds up the sizes of the files that exist among paths.
func FileSizes(paths ...string) uint64 {
	var n uint64
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			n += uint64(st.Size())
		}
	}
	return n
}
