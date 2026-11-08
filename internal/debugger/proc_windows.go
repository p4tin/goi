//go:build windows

package debugger

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
}

func killTree(cmd *exec.Cmd) {
	tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	tk.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	if err := tk.Run(); err != nil {
		_ = cmd.Process.Kill()
	}
}
