//go:build server

package rpc

import (
	"github.com/p4tin/goi/internal/events"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// In server mode (E2E only) every browser tab is a window, but Wails
// broadcasts all events to every tab. A window's events are wrapped in an
// envelope naming the window, and lib/api.ts drops envelopes for other
// windows.

// envelope is a window event as sent in server mode.
type envelope struct {
	Window uint `json:"_w"`
	Data   any  `json:"d"`
}

type windowBus struct {
	app *application.App
	id  func() uint
}

func newWindowBus(application.Window) events.Bus { panic("server mode creates no native windows") }

func (b windowBus) Emit(topic string, payload any) {
	b.app.Event.Emit(topic, envelope{Window: b.id(), Data: payload})
}

// serverHandle is a browser tab; the native window operations don't apply.
type serverHandle struct{}

func (serverHandle) Focus()          {}
func (serverHandle) SetTitle(string) {}
func (serverHandle) Close()          {}

// lazyWindow registers a browser tab as a window on its first call. The
// first tab gets the startup project.
func lazyWindow(a *App, nw application.Window) *window {
	a.winMu.Lock()
	first := len(a.windows) == 0
	a.winMu.Unlock()
	var w *window
	bus := windowBus{app: a.wapp, id: func() uint { return w.id }}
	w, err := a.addWindow(serverHandle{}, bus)
	if err != nil {
		a.log.Error("server window", "err", err)
		return nil
	}
	a.winMu.Lock()
	if prev := a.byNative[nw.ID()]; prev != nil {
		a.winMu.Unlock()
		a.removeWindow(w)
		return prev
	}
	a.byNative[nw.ID()] = w
	a.winMu.Unlock()
	if first {
		a.openStartup(w)
	} else {
		w.ready.Store(true)
	}
	return w
}
