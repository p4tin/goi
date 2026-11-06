package vfs

import (
	"fmt"
	"os/exec"
)

func reveal(path string) error {
	if err := exec.Command("explorer", "/select,"+path).Start(); err != nil {
		return fmt.Errorf("vfs: reveal %s: %w", path, err)
	}
	return nil
}
