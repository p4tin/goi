package vfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const tmpPrefix = ".goi-tmp-"

// atomicWrite writes data to a temp file beside path and renames it over
// path, preserving the existing file mode (0644 for new files).
func atomicWrite(path string, data []byte) (err error) {
	mode := fs.FileMode(0o644)
	if st, statErr := os.Stat(path); statErr == nil {
		mode = st.Mode().Perm()
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("vfs: write: %w", statErr)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), tmpPrefix+"*")
	if err != nil {
		return fmt.Errorf("vfs: write: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("vfs: write: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("vfs: write: sync: %w", err)
	}
	if err = tmp.Chmod(mode); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("vfs: write: chmod: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("vfs: write: close: %w", err)
	}
	if err = renameReplace(tmpName, path); err != nil {
		return fmt.Errorf("vfs: write: rename: %w", err)
	}
	return nil
}

// renameReplace renames from over to. On Windows a concurrent reader (AV,
// indexer) can briefly hold the target, so the rename is retried.
func renameReplace(from, to string) error {
	err := os.Rename(from, to)
	if err == nil || runtime.GOOS != "windows" {
		return err
	}
	for i := 0; i < 5; i++ {
		time.Sleep(20 * time.Millisecond)
		if err = os.Rename(from, to); err == nil {
			return nil
		}
	}
	return err
}
