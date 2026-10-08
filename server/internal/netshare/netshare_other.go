//go:build !windows

package netshare

// Supported: on Linux, macOS and in containers the system mounts shares.
const Supported = false

func Connect(path, user, password string) error { return ErrUnsupported }
func Disconnect(path string) error              { return ErrUnsupported }
func Protect(password string) ([]byte, error)   { return nil, ErrUnsupported }
func Unprotect(secret []byte) (string, error)   { return "", ErrUnsupported }
