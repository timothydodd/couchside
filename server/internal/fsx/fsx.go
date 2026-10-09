// Package fsx holds the file-system checks the scanner and the API share
// about links planted in a media folder.
package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrRefused means a path leads, through a link, to a file Couchside won't
// read: something outside the library that isn't the kind of file asked for.
var ErrRefused = errors.New("refused: the link leads outside the library")

// Inside reports whether p is strictly inside dir. Both are compared as
// given; resolve links first when that matters.
func Inside(p, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(p))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Allowed says whether the file at p may be read on a viewer's behalf. It
// follows any links. A file that really lives under root (resolved too) is
// fine. One that lives elsewhere got there through a link someone put in the
// library: that's fine only for a regular file whose real name ok accepts,
// so "Film.mkv -> /data/auth.key" is refused while a link to a real video on
// another disk (a debrid mount, say) still plays. An empty root applies the
// name rule to every file. A missing file returns the os error.
func Allowed(p, root string, ok func(name string) bool) error {
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return err
	}
	if root != "" {
		if rr, err := filepath.EvalSymlinks(root); err == nil && Inside(real, rr) {
			return nil
		}
	}
	st, err := os.Stat(real)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || !ok(filepath.Base(real)) {
		return ErrRefused
	}
	return nil
}
