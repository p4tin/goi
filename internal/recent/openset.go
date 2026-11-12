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

// OpenSet returns the projects recorded by SetOpenSet; nil if none.
func (s *Store) OpenSet() []string {
	b, err := os.ReadFile(s.openSetPath())
	if err != nil {
		return nil
	}
	var paths []string
	if json.Unmarshal(b, &paths) != nil {
		return nil
	}
	return paths
}
