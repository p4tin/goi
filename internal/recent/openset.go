package recent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// openSetPath is the file next to the recent list that holds the projects
// open when Goi last quit.
func (s *Store) openSetPath() string {
	return filepath.Join(filepath.Dir(s.path), "open-projects.json")
}

// SetOpenSet records the projects open now, in window order.
func (s *Store) SetOpenSet(paths []string) error {
	if paths == nil {
		paths = []string{}
	}
	b, err := json.Marshal(paths)
	if err != nil {
		return fmt.Errorf("recent: encode open set: %w", err)
	}
	if err := s.lock.Lock(); err != nil {
		return fmt.Errorf("recent: lock: %w", err)
	}
	defer s.lock.Unlock()
	return writeAtomic(s.openSetPath(), b)
}

// OpenSet returns the projects recorded by SetOpenSet. ok is false when no
// set was recorded; an empty recorded set means Goi quit from Welcome.
func (s *Store) OpenSet() (paths []string, ok bool) {
	b, err := os.ReadFile(s.openSetPath())
	if err != nil {
		return nil, false
	}
	if json.Unmarshal(b, &paths) != nil {
		return nil, false
	}
	if paths == nil {
		paths = []string{}
	}
	return paths, true
}
