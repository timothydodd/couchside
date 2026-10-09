// Package diskfree reports how much room a folder's disk has.
package diskfree

// Space is a disk's free and total bytes, as the server's user sees them.
type Space struct {
	Free  uint64 `json:"free"`
	Total uint64 `json:"total"`
}

// Of returns the space on the disk holding path.
func Of(path string) (Space, error) { return of(path) }
