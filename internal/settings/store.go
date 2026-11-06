package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"

	"github.com/p4tin/goi/internal/events"
)

// Store is the layered settings Provider backed by TOML files.
type Store struct {
	bus events.Bus

	mu          sync.RWMutex
	paths       map[Layer]string
	docs        map[Layer]map[string]any // last good parsed document per layer
	effective   Settings
	origins     map[string]Layer
	subscribers map[int]func(Settings)
	nextSub     int
}

var _ Provider = (*Store)(nil)

// Load reads defaults, the user file and (optionally) the project file.
// Missing files are treated as empty layers; an invalid user or project file
// is reported through a notification and treated as empty.
func Load(userPath, projectPath string, bus events.Bus) (*Store, error) {
	if bus == nil {
		bus = events.Nop{}
	}
	defaults := map[string]any{}
	if err := toml.Unmarshal(defaultsTOML, &defaults); err != nil {
		return nil, fmt.Errorf("settings: defaults: %w", err)
	}
	s := &Store{
		bus:         bus,
		paths:       map[Layer]string{LayerUser: userPath, LayerProject: projectPath},
		docs:        map[Layer]map[string]any{LayerDefault: defaults, LayerUser: {}, LayerProject: {}},
		subscribers: map[int]func(Settings){},
	}
	for _, l := range []Layer{LayerUser, LayerProject} {
		doc, err := s.readLayer(l)
		if err != nil {
			s.notifyError(l, err)
			continue
		}
		s.docs[l] = doc
	}
	if err := s.recompute(); err != nil {
		return nil, err
	}
	return s, nil
}

// Effective returns the merged settings.
func (s *Store) Effective() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.effective
}

// Origins returns the layer of every leaf key.
func (s *Store) Origins() map[string]Layer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Layer, len(s.origins))
	for k, v := range s.origins {
		out[k] = v
	}
	return out
}

// Origin returns the layer providing key. For a key with children (for
// example "go.env") it returns the highest-priority layer among them; for an
// unknown key it falls back to the nearest known ancestor, then LayerDefault.
func (s *Store) Origin(key string) Layer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if l, ok := s.origins[key]; ok {
		return l
	}
	best, found := LayerDefault, false
	for k, l := range s.origins {
		if strings.HasPrefix(k, key+".") && (!found || l.priority() > best.priority()) {
			best, found = l, true
		}
	}
	if found {
		return best
	}
	for k := key; strings.Contains(k, "."); {
		k = k[:strings.LastIndex(k, ".")]
		if l, ok := s.origins[k]; ok {
			return l
		}
	}
	return LayerDefault
}

// Set writes value at the dotted key in the given layer's file. A nil value
// removes the key from that layer.
func (s *Store) Set(layer Layer, key string, value any) error {
	if layer != LayerUser && layer != LayerProject {
		return fmt.Errorf("settings: cannot set %q in layer %q", key, layer)
	}
	if key == "" {
		return errors.New("settings: empty key")
	}
	if (key == "keys" || strings.HasPrefix(key, "keys.")) && layer != LayerUser {
		return fmt.Errorf("settings: %q is user-only", key)
	}

	s.mu.Lock()
	path := s.paths[layer]
	if path == "" {
		s.mu.Unlock()
		return fmt.Errorf("settings: no file for layer %q", layer)
	}
	doc := deepCopy(s.docs[layer]).(map[string]any)
	setPath(doc, splitKey(key), normalize(value))

	prev := s.docs[layer]
	s.docs[layer] = doc
	if err := s.recomputeLocked(); err != nil {
		s.docs[layer] = prev
		s.mu.Unlock()
		return err
	}
	if err := writeTOML(path, doc); err != nil {
		s.docs[layer] = prev
		s.recomputeLocked()
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	s.changed(layer)
	return nil
}

// splitKey splits a dotted key into path parts. Everything after "keys." is
// one part because command ids contain dots.
func splitKey(key string) []string {
	if rest, ok := strings.CutPrefix(key, "keys."); ok {
		return []string{"keys", rest}
	}
	return strings.Split(key, ".")
}

// Reload re-reads one layer's file. On invalid content the last good layer
// is kept, a notification is emitted and the error is returned.
func (s *Store) Reload(layer Layer) error {
	if layer != LayerUser && layer != LayerProject {
		return fmt.Errorf("settings: cannot reload layer %q", layer)
	}
	doc, err := s.readLayer(layer)
	if err != nil {
		s.notifyError(layer, err)
		return err
	}
	s.mu.Lock()
	prev := s.docs[layer]
	s.docs[layer] = doc
	if err := s.recomputeLocked(); err != nil {
		s.docs[layer] = prev
		s.recomputeLocked()
		s.mu.Unlock()
		s.notifyError(layer, err)
		return err
	}
	s.mu.Unlock()
	s.changed(layer)
	return nil
}

// SetProjectPath switches the project layer to path ("" clears it).
func (s *Store) SetProjectPath(path string) error {
	s.mu.Lock()
	s.paths[LayerProject] = path
	s.mu.Unlock()
	doc, err := s.readLayer(LayerProject)
	if err != nil {
		s.notifyError(LayerProject, err)
		doc = map[string]any{}
	}
	s.mu.Lock()
	s.docs[LayerProject] = doc
	if err := s.recomputeLocked(); err != nil {
		s.docs[LayerProject] = map[string]any{}
		s.recomputeLocked()
		s.mu.Unlock()
		s.notifyError(LayerProject, err)
		return err
	}
	s.mu.Unlock()
	s.changed(LayerProject)
	return nil
}

// Path returns the file backing a layer ("" if none).
func (s *Store) Path(layer Layer) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paths[layer]
}

// Subscribe registers fn to be called with the new effective settings after
// every successful change.
func (s *Store) Subscribe(fn func(Settings)) (cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextSub
	s.nextSub++
	s.subscribers[id] = fn
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subscribers, id)
	}
}

func (s *Store) recompute() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recomputeLocked()
}

func (s *Store) recomputeLocked() error {
	merged := map[string]any{}
	prov := map[string]Layer{}
	for _, l := range []Layer{LayerDefault, LayerUser, LayerProject} {
		merge(merged, s.docs[l], l, "", prov)
	}
	eff, err := decode(merged)
	if err != nil {
		return err
	}
	s.effective, s.origins = eff, prov
	return nil
}

func (s *Store) changed(layer Layer) {
	s.mu.RLock()
	eff := s.effective
	subs := make([]func(Settings), 0, len(s.subscribers))
	for _, fn := range s.subscribers {
		subs = append(subs, fn)
	}
	s.mu.RUnlock()
	for _, fn := range subs {
		fn(eff)
	}
	s.bus.Emit(events.TopicSettingsChanged, ChangedEvent{Layer: layer, Effective: eff})
}

// readLayer parses the layer's file. A missing file or empty path yields an
// empty document.
func (s *Store) readLayer(layer Layer) (map[string]any, error) {
	s.mu.RLock()
	path := s.paths[layer]
	s.mu.RUnlock()
	doc := map[string]any{}
	if path == "" {
		return doc, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return nil, &FileError{File: path, Err: err}
	}
	if err := toml.Unmarshal(b, &doc); err != nil {
		fe := &FileError{File: path, Err: err}
		var de *toml.DecodeError
		if errors.As(err, &de) {
			fe.Line, _ = de.Position()
		}
		return nil, fe
	}
	return doc, nil
}

// FileError is an error reading or parsing a settings file.
type FileError struct {
	File string
	Line int
	Err  error
}

func (e *FileError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("settings: %s:%d: %v", e.File, e.Line, e.Err)
	}
	return fmt.Sprintf("settings: %s: %v", e.File, e.Err)
}

func (e *FileError) Unwrap() error { return e.Err }

func (s *Store) notifyError(layer Layer, err error) {
	n := events.Notification{
		ID:    "settings.invalid." + string(layer),
		Level: "error",
		Title: "Invalid settings file",
		Body:  err.Error(),
	}
	var fe *FileError
	if errors.As(err, &fe) {
		n.File, n.Line = fe.File, fe.Line
	} else {
		n.File = s.Path(layer)
	}
	s.bus.Emit(events.TopicNotify, n)
}

func writeTOML(path string, doc map[string]any) error {
	b, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("settings: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("settings: temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("settings: close: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("settings: rename: %w", err)
	}
	return nil
}
