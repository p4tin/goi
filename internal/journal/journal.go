package journal

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const lockName = "session.lock"

// ErrInUse means another live Goi process owns this workspace's journal.
var ErrInUse = errors.New("journal: workspace journal in use by another process")

// Entry is the journaled content of one unsaved buffer.
type Entry struct {
	Path        string    `json:"path"`
	Content     string    `json:"content"`
	BaseModTime time.Time `json:"baseModTime"`
	SavedAt     time.Time `json:"savedAt"`
}

// Journal stores unsaved buffers for one workspace.
type Journal struct {
	dir string
}

// Open opens the journal for wsHash under cacheDir. crashed reports that a
// previous session exited without CloseClean, so Pending may hold buffers to
// restore.
func Open(cacheDir, wsHash string) (j *Journal, crashed bool, err error) {
	dir := filepath.Join(cacheDir, "journal", wsHash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, false, fmt.Errorf("journal: mkdir: %w", err)
	}
	lock := filepath.Join(dir, lockName)
	b, err := os.ReadFile(lock)
	switch {
	case err == nil:
		pid, perr := strconv.Atoi(strings.TrimSpace(string(b)))
		if perr == nil && pid != os.Getpid() {
			if pidAlive(pid) {
				return nil, false, ErrInUse
			}
			crashed = true
		} else if perr != nil {
			crashed = true
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, false, fmt.Errorf("journal: read lock: %w", err)
	}
	if err := writeAtomic(lock, []byte(strconv.Itoa(os.Getpid()))); err != nil {
		return nil, false, err
	}
	return &Journal{dir: dir}, crashed, nil
}

func (j *Journal) file(path string) string {
	sum := sha1.Sum([]byte(path))
	return filepath.Join(j.dir, hex.EncodeToString(sum[:])+".json")
}

// Put stores or replaces the entry for e.Path.
func (j *Journal) Put(e Entry) error {
	if e.SavedAt.IsZero() {
		e.SavedAt = time.Now()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("journal: encode: %w", err)
	}
	return writeAtomic(j.file(e.Path), b)
}

// Drop removes the entry for path, if any.
func (j *Journal) Drop(path string) error {
	if err := os.Remove(j.file(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("journal: drop: %w", err)
	}
	return nil
}

// Pending returns all journaled entries sorted by path.
func (j *Journal) Pending() ([]Entry, error) {
	files, err := filepath.Glob(filepath.Join(j.dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("journal: list: %w", err)
	}
	var out []Entry
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("journal: read: %w", err)
		}
		var e Entry
		if err := json.Unmarshal(b, &e); err != nil {
			continue // a torn or foreign file is not restorable; skip it
		}
		out = append(out, e)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Path < out[b].Path })
	return out, nil
}

// CloseClean removes all entries and the session lock after a clean exit.
func (j *Journal) CloseClean() error {
	files, err := filepath.Glob(filepath.Join(j.dir, "*.json"))
	if err != nil {
		return fmt.Errorf("journal: list: %w", err)
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("journal: remove: %w", err)
		}
	}
	if err := os.Remove(filepath.Join(j.dir, lockName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("journal: remove lock: %w", err)
	}
	return nil
}

func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".goi-tmp-*")
	if err != nil {
		return fmt.Errorf("journal: temp: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("journal: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("journal: close: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return fmt.Errorf("journal: rename: %w", err)
	}
	return nil
}
