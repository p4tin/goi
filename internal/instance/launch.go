package instance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// LaunchNewWindow starts another Goi process opening projectPath. It does
// not wait for the process.
func LaunchNewWindow(projectPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("instance: executable: %w", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	name, args := launchCommand(runtime.GOOS, exe, projectPath)
	cmd := exec.Command(name, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("instance: launch: %w", err)
	}
	go cmd.Wait() // reap
	return nil
}

// launchCommand returns the program and arguments that open project in a new
// Goi window. On macOS, binaries inside an .app bundle launch through `open
// -n` so the window joins the app's Dock icon.
func launchCommand(goos, exe, project string) (string, []string) {
	if goos == "darwin" {
		const marker = ".app/Contents/MacOS/"
		if i := strings.Index(exe, marker); i >= 0 {
			bundle := exe[:i+len(".app")]
			return "open", []string{"-n", "-a", bundle, "--args", "--project", project}
		}
	}
	return exe, []string{"--project", project}
}
