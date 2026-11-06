package vfs

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func trash(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("vfs: trash: %w", err)
	}
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(abs)
	cmd := exec.Command("osascript", "-e", `tell application "Finder" to delete POSIX file "`+esc+`"`)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vfs: trash %s: %w: %s", abs, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
