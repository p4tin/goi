package rpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/instance"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/vfs"
	"github.com/p4tin/goi/internal/workspace"
)

// windowHandle is the native window behind a window; tests use a fake.
type windowHandle interface {
	// Focus unminimises, shows and focuses the window.
	Focus()
	SetTitle(title string)
}

// window is the per-window state: its event bus, settings (user layer plus
// its project's layer), the project session (nil on Welcome) and the close
// handshake.
type window struct {
	id       uint
	app      *App
	handle   windowHandle
	bus      events.Bus
	settings *settings.Store
	cfgWatch vfs.Watcher // watches the settings files; no content roots

	mu      sync.Mutex
	session *session

	closing closeState
}

// addWindow registers a window that delivers its events through bus, with its
// own settings store.
func (a *App) addWindow(handle windowHandle, bus events.Bus) (*window, error) {
	st, err := settings.Load(filepath.Join(a.configDir, "settings.toml"), "", bus)
	if err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	w := &window{app: a, handle: handle, bus: bus, settings: st}
	if w.cfgWatch, err = vfs.NewWatcher(nil, vfs.MatchNone, events.Nop{}); err != nil {
		a.log.Warn("config watcher", "err", err)
	} else if _, err := st.Watch(w.cfgWatch); err != nil {
		a.log.Warn("watch settings", "err", err)
	}
	a.winMu.Lock()
	a.nextWindow++
	w.id = a.nextWindow
	a.windows[w.id] = w
	a.winMu.Unlock()
	return w, nil
}

// removeWindow unregisters w and releases its session and watchers.
func (a *App) removeWindow(w *window) {
	a.winMu.Lock()
	delete(a.windows, w.id)
	a.winMu.Unlock()
	w.mu.Lock()
	s := w.session
	w.session = nil
	w.mu.Unlock()
	if s != nil {
		s.close(true)
	}
	if w.cfgWatch != nil {
		w.cfgWatch.Close()
	}
}

// allWindows returns the registered windows in creation order.
func (a *App) allWindows() []*window {
	a.winMu.Lock()
	defer a.winMu.Unlock()
	out := make([]*window, 0, len(a.windows))
	for _, w := range a.windows {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// mainWindow returns the first registered window, or nil.
//
// Transitional: bound methods use it until they take a context and resolve
// the calling window with windowFor (Wails v3).
func (a *App) mainWindow() *window {
	ws := a.allWindows()
	if len(ws) == 0 {
		return nil
	}
	return ws[0]
}

type windowCtxKey struct{}

// withWindow returns a context whose bound calls act on w.
func withWindow(ctx context.Context, w *window) context.Context {
	return context.WithValue(ctx, windowCtxKey{}, w)
}

var errNoWindow = errors.New("no window")

// windowFor returns the window that made the call. A context without a window
// (tests) falls back to the only window when exactly one exists.
func (a *App) windowFor(ctx context.Context) (*window, error) {
	if ctx != nil {
		if w, ok := ctx.Value(windowCtxKey{}).(*window); ok && w != nil {
			return w, nil
		}
	}
	ws := a.allWindows()
	if len(ws) == 1 {
		return ws[0], nil
	}
	return nil, errNoWindow
}

// sessionFor returns the project session of the calling window.
func (a *App) sessionFor(ctx context.Context) (*session, error) {
	w, err := a.windowFor(ctx)
	if err != nil {
		return nil, err
	}
	return w.current()
}

// current returns the window's session or errNoProject.
func (w *window) current() (*session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.session == nil {
		return nil, errNoProject
	}
	return w.session, nil
}

func (w *window) focus() {
	if w.handle != nil {
		w.handle.Focus()
	}
}

// setTitle sets the window title, if there is a native window.
func (w *window) setTitle(title string) {
	if w.handle != nil {
		w.handle.SetTitle(title)
	}
}

func (w *window) notify(level, title, body string) {
	w.bus.Emit(events.TopicNotify, events.Notification{ID: title, Level: level, Title: title, Body: body})
}

// settingsLayerForProject picks the project layer when a project is open,
// else the user layer.
func (w *window) settingsLayerForProject() settings.Layer {
	if _, err := w.current(); err == nil {
		return settings.LayerProject
	}
	return settings.LayerUser
}

// ErrOpenElsewhere means the project is open in another Goi window, which
// has been asked to come to the front.
var ErrOpenElsewhere = errors.New("project is already open in another Goi window")

// launchNewWindow starts a new Goi process; tests replace it.
var launchNewWindow = instance.LaunchNewWindow

// acquireInstance takes the per-project instance lock. If another live
// window holds it, that window is focused and ErrOpenElsewhere is returned.
func (w *window) acquireInstance(root string) (*instance.Lock, error) {
	a := w.app
	hash := workspace.Hash(root)
	lock, err := instance.Acquire(a.cacheDir, hash, w.focus)
	var held *instance.ErrHeld
	if errors.As(err, &held) {
		if ferr := instance.Focus(a.cacheDir, hash); ferr != nil {
			a.log.Warn("focus other window", "err", ferr)
		}
		return nil, fmt.Errorf("open project %s: %w", root, ErrOpenElsewhere)
	}
	if err != nil {
		return nil, fmt.Errorf("open project: %w", err)
	}
	return lock, nil
}

// openInOtherWindow focuses the window that already has path open, or
// launches a new Goi window for it.
func (w *window) openInOtherWindow(path string) error {
	a := w.app
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("open project: %w", err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return fmt.Errorf("open project: %s is not a folder", abs)
	}
	if sess, err := w.current(); err == nil && sess.root == abs {
		w.focus()
		return nil
	}
	hash := workspace.Hash(abs)
	if instance.Held(a.cacheDir, hash) {
		return instance.Focus(a.cacheDir, hash)
	}
	a.log.Info("launch new window", "project", abs)
	return launchNewWindow(abs)
}
