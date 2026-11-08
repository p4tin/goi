//go:build windows

package process

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// A GUI process can't deliver Ctrl-Break to children started without a
// console, so Stop kills the tree.
const canInterrupt = false

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// setGroup starts the process in a new process group without a console
// window popping up.
func setGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | createNoWindow, HideWindow: true}
}

func interruptGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

// killGroup kills the process and its descendants.
func killGroup(cmd *exec.Cmd) {
	tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	tk.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	if err := tk.Run(); err != nil {
		_ = cmd.Process.Kill()
	}
}

func signalOf(*os.ProcessState) string { return "" }
