package vfs

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// trashFreedesktop moves path into dataHome/Trash per the FreeDesktop.org
// Trash specification. It is OS-independent so it can be tested everywhere.
func trashFreedesktop(path, dataHome string, now time.Time) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("vfs: trash: %w", err)
	}
	if _, err := os.Lstat(abs); err != nil {
		return fmt.Errorf("vfs: trash %s: %w", abs, err)
	}
	filesDir := filepath.Join(dataHome, "Trash", "files")
	infoDir := filepath.Join(dataHome, "Trash", "info")
	for _, d := range []string{filesDir, infoDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("%w: %v", ErrTrashUnavailable, err)
		}
	}
	info := "[Trash Info]\nPath=" + encodeTrashPath(abs) + "\nDeletionDate=" + now.Format("2006-01-02T15:04:05") + "\n"

	base := filepath.Base(abs)
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = base + "." + strconv.Itoa(n)
		}
		infoPath := filepath.Join(infoDir, name+".trashinfo")
		f, err := os.OpenFile(infoPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("vfs: trash: %w", err)
		}
		_, werr := f.WriteString(info)
		cerr := f.Close()
		if werr != nil || cerr != nil {
			os.Remove(infoPath)
			return fmt.Errorf("vfs: trash: write info: %w", errors.Join(werr, cerr))
		}
		dest := filepath.Join(filesDir, name)
		if _, err := os.Lstat(dest); err == nil {
			os.Remove(infoPath)
			continue
		}
		if err := os.Rename(abs, dest); err != nil {
			os.Remove(infoPath)
			if errors.Is(err, syscall.EXDEV) {
				return fmt.Errorf("%w: %s is on a different filesystem", ErrTrashUnavailable, abs)
			}
			return fmt.Errorf("vfs: trash %s: %w", abs, err)
		}
		return nil
	}
}

// encodeTrashPath percent-encodes each path segment, keeping "/" separators.
func encodeTrashPath(p string) string {
	segs := strings.Split(filepath.ToSlash(p), "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
