package runconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// MaxTemporary is how many temporary configurations are kept.
const MaxTemporary = 5

// Store holds the configurations of one project: shared ones in the
// project's .goi/runConfigurations and local ones in the user config dir.
type Store struct {
	sharedDir, localDir string

	mu    sync.Mutex
	cfgs  map[string]Config // by key(name)
	files map[string]string // key(name) → file path
	// LoadErrors lists files that could not be parsed on the last load.
	loadErrors []string
}

// Open loads the configurations in both directories. Missing directories
// are fine.
func Open(sharedDir, localDir string) (*Store, error) {
	s := &Store{sharedDir: sharedDir, localDir: localDir}
	return s, s.Reload()
}

// SharedDir is the project's run configuration directory.
func (s *Store) SharedDir() string { return s.sharedDir }

// LocalDir is the user-local run configuration directory.
func (s *Store) LocalDir() string { return s.localDir }

// Reload re-reads both directories. Unparseable files are skipped and
// reported by LoadErrors.
func (s *Store) Reload() error {
	cfgs := map[string]Config{}
	files := map[string]string{}
	var bad []string
	for _, d := range []struct {
		dir    string
		shared bool
	}{{s.localDir, false}, {s.sharedDir, true}} {
		if d.dir == "" {
			continue
		}
		entries, err := os.ReadDir(d.dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("runconfig: read %s: %w", d.dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
				continue
			}
			p := filepath.Join(d.dir, e.Name())
			c, err := readFile(p)
			if err != nil {
				bad = append(bad, fmt.Sprintf("%s: %v", p, err))
				continue
			}
			c.Shared = d.shared
			if c.Shared {
				c.Temporary = false
			}
			// Shared wins over a local configuration with the same name.
			cfgs[key(c.Name)] = c
			files[key(c.Name)] = p
		}
	}
	s.mu.Lock()
	s.cfgs, s.files, s.loadErrors = cfgs, files, bad
	s.mu.Unlock()
	return nil
}

// LoadErrors returns the files skipped by the last load.
func (s *Store) LoadErrors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.loadErrors...)
}

func readFile(p string) (Config, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := toml.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(c.Name) == "" {
		c.Name = strings.TrimSuffix(filepath.Base(p), ".toml")
	}
	return c.Normalize(), nil
}

// List returns permanent configurations sorted by name, then temporary ones,
// newest first.
func (s *Store) List() []Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Config, 0, len(s.cfgs))
	for _, c := range s.cfgs {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Temporary != b.Temporary {
			return !a.Temporary
		}
		if a.Temporary {
			return a.Created > b.Created
		}
		return key(a.Name) < key(b.Name)
	})
	return out
}

// Get returns the configuration named name.
func (s *Store) Get(name string) (Config, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cfgs[key(name)]
	return c, ok
}

// Save writes c, replacing the configuration called oldName ("" for a new
// one). Renaming onto another existing name fails. A shared configuration
// is never temporary.
func (s *Store) Save(c Config, oldName string) (Config, error) {
	c = c.Normalize()
	if c.Name == "" {
		return Config{}, errors.New("runconfig: name is required")
	}
	if c.Shared {
		c.Temporary = false
		c.Created = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cfgs[key(c.Name)]; ok && key(c.Name) != key(oldName) {
		return Config{}, fmt.Errorf("runconfig: a configuration named %q already exists", c.Name)
	}
	if c.Shared && s.sharedDir == "" {
		return Config{}, errors.New("runconfig: no project directory for shared configurations")
	}
	dir := s.localDir
	if c.Shared {
		dir = s.sharedDir
	}
	p := filepath.Join(dir, Slug(c.Name)+".toml")
	// Avoid clobbering a different configuration whose name slugs the same.
	for k, f := range s.files {
		if f == p && k != key(oldName) && k != key(c.Name) {
			p = filepath.Join(dir, fmt.Sprintf("%s-%d.toml", Slug(c.Name), time.Now().UnixNano()))
			break
		}
	}
	b, err := toml.Marshal(c)
	if err != nil {
		return Config{}, fmt.Errorf("runconfig: encode: %w", err)
	}
	if err := writeAtomic(p, b); err != nil {
		return Config{}, fmt.Errorf("runconfig: save %s: %w", c.Name, err)
	}
	if oldName != "" {
		if old, ok := s.files[key(oldName)]; ok && old != p {
			os.Remove(old)
		}
		delete(s.cfgs, key(oldName))
		delete(s.files, key(oldName))
	}
	s.cfgs[key(c.Name)] = c
	s.files[key(c.Name)] = p
	return c, nil
}

// Delete removes the configuration named name.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.files[key(name)]
	if !ok {
		return fmt.Errorf("runconfig: no configuration named %q", name)
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("runconfig: delete %s: %w", name, err)
	}
	delete(s.cfgs, key(name))
	delete(s.files, key(name))
	return nil
}

// AddTemporary stores c as a temporary configuration and returns it. An
// existing temporary configuration with the same name is replaced; a
// permanent one with the same name gets the new one renamed "name (2)".
// Only the newest MaxTemporary temporary configurations are kept.
func (s *Store) AddTemporary(c Config) (Config, error) {
	c.Temporary, c.Shared = true, false
	c.Created = time.Now().UnixMilli()
	for _, o := range s.List() {
		if o.Temporary && o.Created >= c.Created {
			c.Created = o.Created + 1 // keep creation order strict
		}
	}
	c = c.Normalize()
	if existing, ok := s.Get(c.Name); ok && existing.Temporary {
		// Keep identical settings' history position fresh.
		saved, err := s.Save(c, existing.Name)
		if err != nil {
			return Config{}, err
		}
		s.prune()
		return saved, nil
	}
	base := c.Name
	for i := 2; ; i++ {
		if _, ok := s.Get(c.Name); !ok {
			break
		}
		c.Name = fmt.Sprintf("%s (%d)", base, i)
	}
	saved, err := s.Save(c, "")
	if err != nil {
		return Config{}, err
	}
	s.prune()
	return saved, nil
}

func (s *Store) prune() {
	var temps []Config
	for _, c := range s.List() {
		if c.Temporary {
			temps = append(temps, c)
		}
	}
	for i := MaxTemporary; i < len(temps); i++ {
		_ = s.Delete(temps[i].Name)
	}
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".goi-tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
