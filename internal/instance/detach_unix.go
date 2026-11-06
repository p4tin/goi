//go:build !windows

package instance

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in its own session so it outlives this process.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
