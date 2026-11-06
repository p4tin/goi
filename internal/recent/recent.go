package recent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// MaxEntries is the maximum number of recent projects kept.
const MaxEntries = 20

// Entry is one recent project.
type Entry struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	OpenedAt time.Time `json:"openedAt"`
	Exists   bool      `json:"exists"`
}

// Store persists the recent projects list in a JSON file shared by all Goi
// processes. Every operation re-reads the file under a lock, so changes made
// by other processes are never lost.
type Store struct {
	path string
	lock *flock.Flock
}

// Load opens the store backed by path. A missing file is an empty list.
func Load(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("recent: mkdir: %w", err)
	}
	s := &Store{path: path, lock: flock.New(path + ".lock")}
	if _, err := s.List(); err != nil {
		return nil, err
	}
	return s, nil
}

// Touch moves e to the top of the list, adding it if absent.
func (s *Store) Touch(e Entry) error {
	e.Path = filepath.Clean(e.Path)
	if e.Name == "" {
		e.Name = filepath.Base(e.Path)
	}
	if e.OpenedAt.IsZero() {
		e.OpenedAt = time.Now()
	}
	e.Exists = false
	return s.modify(func(list []Entry) []Entry {
		out := make([]Entry, 0, len(list)+1)
		out = append(out, e)
		for _, x := range list {
			if filepath.Clean(x.Path) != e.Path {
				out = append(out, x)
			}
		}
		if len(out) > MaxEntries {
			out = out[:MaxEntries]
		}
		return out
	})
}

// Remove deletes path from the list.
func (s *Store) Remove(path string) error {
	path = filepath.Clean(path)
	return s.modify(func(list []Entry) []Entry {
		out := list[:0]
		for _, x := range list {
			if filepath.Clean(x.Path) != path {
				out = append(out, x)
			}
		}
		return out
	})
}

// List returns the entries, most recent first, with Exists filled in.
func (s *Store) List() ([]Entry, error) {
	if err := s.lock.RLock(); err != nil {
		return nil, fmt.Errorf("recent: lock: %w", err)
	}
	list, err := s.read()
	s.lock.Unlock()
	if err != nil {
		return nil, err
	}
	for i := range list {
		st, err := os.Stat(list[i].Path)
		list[i].Exists = err == nil && st.IsDir()
	}
	return list, nil
}

func (s *Store) modify(fn func([]Entry) []Entry) error {
	if err := s.lock.Lock(); err != nil {
		return fmt.Errorf("recent: lock: %w", err)
	}
	defer s.lock.Unlock()
	list, err := s.read()
	if err != nil {
		return err
	}
	return s.write(fn(list))
}

func (s *Store) read() ([]Entry, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("recent: read: %w", err)
	}
	var list []Entry
	if len(b) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("recent: decode %s: %w", s.path, err)
	}
	return list, nil
}

func (s *Store) write(list []Entry) error {
	if list == nil {
		list = []Entry{}
	}
	for i := range list {
		list[i].Exists = false
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("recent: encode: %w", err)
	}
	return writeAtomic(s.path, b)
}

func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".goi-tmp-*")
	if err != nil {
		return fmt.Errorf("recent: temp: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("recent: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("recent: close: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return fmt.Errorf("recent: rename: %w", err)
	}
	return nil
}
