//go:build windows

package transcode

import "os"

// No SIGSTOP on Windows: ffmpeg just runs ahead (dev builds only).
func suspend(*os.Process) {}
func resume(*os.Process)  {}
