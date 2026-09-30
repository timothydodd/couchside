//go:build !windows

package transcode

import (
	"os"
	"syscall"
)

// suspend/resume throttle an ffmpeg that has run far ahead of the player.
func suspend(p *os.Process) { _ = p.Signal(syscall.SIGSTOP) }
func resume(p *os.Process)  { _ = p.Signal(syscall.SIGCONT) }
