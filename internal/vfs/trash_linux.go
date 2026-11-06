package vfs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func trash(path string) error {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrTrashUnavailable, err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return trashFreedesktop(path, dataHome, time.Now())
}
