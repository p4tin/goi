//go:build !windows

package debugger

import (
	"os/exec"
	"syscall"
)

// setProcAttr puts dlv in its own process group so killing it also stops
// the debuggee it started.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killTree(cmd *exec.Cmd) {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
