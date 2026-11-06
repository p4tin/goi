//go:build !darwin && !windows

package vfs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func reveal(path string) error {
	dir := path
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		dir = filepath.Dir(path)
	}
	if err := exec.Command("xdg-open", dir).Start(); err != nil {
		return fmt.Errorf("vfs: reveal %s: %w", path, err)
	}
	return nil
}
