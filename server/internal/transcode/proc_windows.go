//go:build windows

package transcode

import "os"

// No SIGSTOP on Windows, so ffmpeg isn't paused when it's far ahead of the
// player: a session converts its whole file, using CPU or GPU until it's
// done or goes idle. Releases ship a Windows build, so this is how it runs
// there.
func suspend(*os.Process) {}
func resume(*os.Process)  {}
