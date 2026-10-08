// Package netshare signs in to Windows network shares (SMB) with saved
// credentials, so libraries can live on a NAS that the service's own account
// (Local System) can't read. It's what "net use \\nas\media /user:..." does,
// for the server's process. Elsewhere shares are mounted by the system
// (Docker, Helm, fstab), so Supported is false.
package netshare

import (
	"errors"
	"strings"
)

// Share is a saved sign-in. Secret is the password, sealed by Protect.
type Share struct {
	Path   string `json:"path"` // \\server\share
	User   string `json:"user"`
	Secret []byte `json:"secret,omitempty"`
}

// ErrUnsupported: this platform mounts shares itself.
var ErrUnsupported = errors.New("signing in to network shares is only for Couchside on Windows")

// Root is the \\server\share a path is on: shares are signed in to whole.
func Root(p string) (string, error) {
	p = strings.ReplaceAll(strings.TrimSpace(p), "/", `\`)
	if !strings.HasPrefix(p, `\\`) {
		return "", errors.New(`a network path starts with \\, like \\nas\media`)
	}
	parts := strings.Split(strings.TrimPrefix(p, `\\`), `\`)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", errors.New(`a network path names the computer and the share, like \\nas\media`)
	}
	return `\\` + parts[0] + `\` + parts[1], nil
}

// ConnectAll signs in to every share, returning each one's error ("" when
// connected), keyed by path.
func ConnectAll(shares []Share) map[string]string {
	out := make(map[string]string, len(shares))
	for _, s := range shares {
		pw, err := Unprotect(s.Secret)
		if err == nil {
			err = Connect(s.Path, s.User, pw)
		}
		out[s.Path] = ""
		if err != nil {
			out[s.Path] = err.Error()
		}
	}
	return out
}
