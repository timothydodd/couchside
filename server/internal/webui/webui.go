// Package webui embeds the built web UI so a release binary is self-contained.
//
// Release builds copy web/dist into internal/webui/dist before `go build`.
// A plain `go build` without it embeds only a placeholder, and the server
// then expects COUCHSIDE_WEB_DIR (or serves the API alone).
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// FS returns the embedded UI, or nil when the binary was built without one.
func FS() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
