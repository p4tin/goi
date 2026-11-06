package vfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Entry is one directory listing item.
type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// List returns the entries of dir (absolute), excluding those whose path
// relative to root is matched by m. Directories sort first, then names in
// natural, case-insensitive order. Symlinks report the kind of their target;
// broken symlinks are reported as files.
func List(root, dir string, m Matcher) ([]Entry, error) {
	if m == nil {
		m = MatchNone
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("vfs: list %s: %w", dir, err)
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		p := filepath.Join(dir, de.Name())
		e := Entry{Name: de.Name(), Path: p, IsDir: de.IsDir()}
		if de.Type()&fs.ModeSymlink != 0 {
			if st, err := os.Stat(p); err == nil {
				e.IsDir = st.IsDir()
				e.Size = st.Size()
			} else {
				e.IsDir = false
			}
		} else if !e.IsDir {
			if info, err := de.Info(); err == nil {
				e.Size = info.Size()
			}
		}
		if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
			if m.Match(filepath.ToSlash(rel), e.IsDir) {
				continue
			}
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return naturalLess(out[i].Name, out[j].Name)
	})
	return out, nil
}

// naturalLess compares case-insensitively, treating digit runs as numbers.
func naturalLess(a, b string) bool {
	ar, br := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			si, sj := i, j
			for i < len(ar) && unicode.IsDigit(ar[i]) {
				i++
			}
			for j < len(br) && unicode.IsDigit(br[j]) {
				j++
			}
			na := strings.TrimLeft(string(ar[si:i]), "0")
			nb := strings.TrimLeft(string(br[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ar[i] != br[j] {
			return ar[i] < br[j]
		}
		i++
		j++
	}
	if len(ar)-i != len(br)-j {
		return len(ar)-i < len(br)-j
	}
	return a < b // stable tie-break on original case
}

// Create makes a file with content, or a directory, creating missing parents.
// It fails if path already exists.
func Create(path string, isDir bool, content string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("vfs: create %s: %w", path, fs.ErrExist)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("vfs: create %s: %w", path, err)
	}
	if isDir {
		if err := os.Mkdir(path, 0o755); err != nil {
			return fmt.Errorf("vfs: create %s: %w", path, err)
		}
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("vfs: create %s: %w", path, err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return fmt.Errorf("vfs: create %s: %w", path, err)
	}
	return f.Close()
}

// Rename moves from to to. It fails if to already exists, unless to is the
// same file as from (a case-only rename on a case-insensitive filesystem).
func Rename(from, to string) error {
	if st, err := os.Lstat(to); err == nil {
		src, serr := os.Lstat(from)
		if serr != nil || !os.SameFile(src, st) {
			return fmt.Errorf("vfs: rename to %s: %w", to, fs.ErrExist)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("vfs: rename to %s: %w", to, err)
	}
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("vfs: rename %s: %w", from, err)
	}
	return nil
}
