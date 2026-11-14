package rpc

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/vfs"
)

// windowHandle is the native window behind a window; tests use a fake.
type windowHandle interface {
	// Focus unminimises, shows and focuses the window.
	Focus()
	SetTitle(title string)
	// Close closes the native window through its close hook.
	Close()
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

	// ready is set once the startup project (if any) is open, so the
	// frontend boots into it (AppService.WindowID).
	ready atomic.Bool

	closing closeState

	ui *uiBroker // requests to the frontend (agent file access, IDE tools)
}

// addWindow registers a window that delivers its events through bus, with its
// own settings store.
func (a *App) addWindow(handle windowHandle, bus events.Bus) (*window, error) {
	st, err := settings.Load(filepath.Join(a.configDir, "settings.toml"), "", bus)
	if err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	w := &window{app: a, handle: handle, bus: bus, settings: st}
	w.ui = newUIBroker(func() events.Bus { return w.bus })
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
	if nh, ok := handle.(*nativeHandle); ok {
		a.bindNative(w, nh)
	}
	return w, nil
}

// removeWindow unregisters w and releases its session and watchers.
func (a *App) removeWindow(w *window) {
	a.winMu.Lock()
	delete(a.windows, w.id)
	for id, x := range a.byNative {
		if x == w {
			delete(a.byNative, id)
		}
	}
	a.winMu.Unlock()
	w.ui.close()
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

type windowCtxKey struct{}

// withWindow returns a context whose bound calls act on w.
func withWindow(ctx context.Context, w *window) context.Context {
	return context.WithValue(ctx, windowCtxKey{}, w)
}

var errNoWindow = errors.New("no window")

// windowFor returns the window that made the call. Without a Wails app
// (tests), a context without a window falls back to the only window when
// exactly one exists. With one, a call without a window fails: in server
// mode a tab's first calls can arrive before its window exists, and must
// not be answered for another window.
func (a *App) windowFor(ctx context.Context) (*window, error) {
	if ctx != nil {
		if w, ok := ctx.Value(windowCtxKey{}).(*window); ok && w != nil {
			return w, nil
		}
		if w := a.callerWindow(ctx); w != nil {
			return w, nil
		}
	}
	if a.wapp != nil {
		return nil, errNoWindow
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

// requestClose starts the window's close flow, as if the user closed it.
func (w *window) requestClose() {
	if w.handle != nil {
		w.handle.Close()
		return
	}
	w.beforeClose()
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
