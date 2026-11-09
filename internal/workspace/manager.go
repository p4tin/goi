package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/vfs"
)

// redetectDelay debounces go.mod/go.work changes before re-running Detect.
const redetectDelay = 300 * time.Millisecond

// Options configure Open.
type Options struct {
	Root     string
	GOPATH   []string
	Settings *settings.Store // project layer must already point at Root/.goi/settings.toml
	Watcher  vfs.Watcher     // optional; enables live re-detection
	Bus      events.Bus
	Log      *slog.Logger
}

// Manager owns the open project model and keeps it current as go.mod,
// go.work and project settings change.
type Manager struct {
	opts Options

	mu       sync.RWMutex
	project  Project
	matcher  vfs.Matcher
	lastErrs []ParseError
	timer    *time.Timer
	closed   bool
	// opened is set after Open's first detection. Until then no
	// workspace.changed is emitted: the owner announces the project once it
	// can serve requests for it.
	opened bool

	cancels []func()
	subs    map[int]func(Project)
	subSeq  int
}

// SettingsPath returns the project settings file for root.
func SettingsPath(root string) string {
	return filepath.Join(root, ".goi", "settings.toml")
}

// Open detects the project at opts.Root and starts tracking changes.
func Open(ctx context.Context, opts Options) (*Manager, error) {
	if opts.Bus == nil {
		opts.Bus = events.Nop{}
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	if opts.Settings == nil {
		return nil, fmt.Errorf("workspace: settings store required")
	}
	m := &Manager{opts: opts}
	if err := m.redetect(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.opened = true
	m.mu.Unlock()
	if opts.Watcher != nil {
		m.cancels = append(m.cancels, opts.Watcher.Subscribe(m.onFS))
	}
	m.cancels = append(m.cancels, opts.Settings.Subscribe(func(settings.Settings) {
		m.onSettings()
	}))
	return m, nil
}

// Project returns a copy of the current project model.
func (m *Manager) Project() Project {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p := m.project
	p.Modules = slices.Clone(p.Modules)
	p.ContentRoots = slices.Clone(p.ContentRoots)
	p.Excludes = slices.Clone(p.Excludes)
	p.ParseErrors = slices.Clone(p.ParseErrors)
	return p
}

// Matcher returns the exclude matcher for the current settings.
func (m *Manager) Matcher() vfs.Matcher {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.matcher
}

// AddContentRoot adds an extra top-level folder, persisted in project settings.
func (m *Manager) AddContentRoot(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return fmt.Errorf("workspace: %s is not a directory", abs)
	}
	roots := m.opts.Settings.Effective().Project.ContentRoots
	if abs == m.opts.Root || slices.Contains(roots, abs) {
		return nil
	}
	return m.opts.Settings.Set(settings.LayerProject, "project.contentRoots", append(slices.Clone(roots), abs))
}

// RemoveContentRoot removes an extra folder added by AddContentRoot.
func (m *Manager) RemoveContentRoot(path string) error {
	roots := m.opts.Settings.Effective().Project.ContentRoots
	out := slices.DeleteFunc(slices.Clone(roots), func(r string) bool { return r == path })
	if len(out) == len(roots) {
		return nil
	}
	return m.opts.Settings.Set(settings.LayerProject, "project.contentRoots", out)
}

// SetExcludes replaces the exclude globs, persisted in project settings.
func (m *Manager) SetExcludes(globs []string) error {
	if _, err := vfs.NewMatcher(globs); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	return m.opts.Settings.Set(settings.LayerProject, "project.excludes", globs)
}

// Subscribe calls fn with the new project after every re-detection.
func (m *Manager) Subscribe(fn func(Project)) (cancel func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.subs == nil {
		m.subs = map[int]func(Project){}
	}
	m.subSeq++
	id := m.subSeq
	m.subs[id] = fn
	return func() {
		m.mu.Lock()
		delete(m.subs, id)
		m.mu.Unlock()
	}
}

// Close stops tracking changes.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	if m.timer != nil {
		m.timer.Stop()
	}
	cancels := m.cancels
	m.cancels = nil
	m.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

func (m *Manager) onFS(evs []vfs.Event) {
	for _, e := range evs {
		if name := filepath.Base(e.Path); name == "go.mod" || name == "go.work" {
			m.scheduleRedetect()
			return
		}
	}
}

func (m *Manager) onSettings() {
	s := m.opts.Settings.Effective().Project
	m.mu.RLock()
	same := slices.Equal(s.Excludes, m.project.Excludes) && slices.Equal(m.extraRoots(s.ContentRoots), m.project.ContentRoots[1:])
	m.mu.RUnlock()
	if !same {
		m.scheduleRedetect()
	}
}

func (m *Manager) scheduleRedetect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(redetectDelay, func() {
		if err := m.redetect(context.Background()); err != nil {
			m.opts.Log.Warn("workspace redetect", "err", err)
		}
	})
}

// redetect recomputes the project from disk and settings and publishes it.
func (m *Manager) redetect(ctx context.Context) error {
	s := m.opts.Settings.Effective().Project
	matcher, err := vfs.NewMatcher(s.Excludes)
	if err != nil {
		m.opts.Bus.Emit(events.TopicNotify, events.Notification{
			ID: "workspace.excludes", Level: "error", Title: "Invalid exclude pattern", Body: err.Error(),
		})
		matcher = vfs.MatchNone
	}
	p, err := Detect(ctx, m.opts.Root, m.opts.GOPATH, matcher)
	if err != nil {
		return err
	}
	p.Excludes = slices.Clone(s.Excludes)
	if p.Excludes == nil {
		p.Excludes = []string{}
	}
	p.ContentRoots = append(p.ContentRoots, m.extraRoots(s.ContentRoots)...)

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	oldRoots := m.project.ContentRoots
	m.project, m.matcher = p, matcher
	errsChanged := !slices.Equal(p.ParseErrors, m.lastErrs)
	m.lastErrs = p.ParseErrors
	subs := make([]func(Project), 0, len(m.subs))
	for _, fn := range m.subs {
		subs = append(subs, fn)
	}
	m.mu.Unlock()
	for _, fn := range subs {
		fn(p)
	}

	if w := m.opts.Watcher; w != nil {
		for _, r := range p.ContentRoots[1:] {
			if !slices.Contains(oldRoots, r) {
				if err := w.AddRoot(r); err != nil {
					m.opts.Log.Warn("watch content root", "root", r, "err", err)
				}
			}
		}
		for _, r := range oldRoots {
			if r != p.Root && !slices.Contains(p.ContentRoots, r) {
				w.RemoveRoot(r)
			}
		}
	}
	if errsChanged {
		for _, pe := range p.ParseErrors {
			m.opts.Bus.Emit(events.TopicNotify, events.Notification{
				ID:    "workspace.parse:" + pe.File,
				Level: "error",
				Title: "Cannot parse " + filepath.Base(pe.File),
				Body:  pe.Msg,
				File:  pe.File,
				Line:  pe.Line,
			})
		}
	}
	m.mu.RLock()
	opened := m.opened
	m.mu.RUnlock()
	if opened {
		m.opts.Bus.Emit(events.TopicWorkspaceChanged, p)
	}
	return nil
}

// extraRoots filters configured content roots to existing directories other
// than the project root.
func (m *Manager) extraRoots(configured []string) []string {
	out := []string{}
	for _, r := range configured {
		if !filepath.IsAbs(r) {
			r = filepath.Join(m.opts.Root, r)
		}
		r = filepath.Clean(r)
		if r == m.opts.Root || slices.Contains(out, r) {
			continue
		}
		if st, err := os.Stat(r); err == nil && st.IsDir() {
			out = append(out, r)
		}
	}
	return out
}
