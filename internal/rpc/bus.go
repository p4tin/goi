//go:build !server

package rpc

import (
	"github.com/p4tin/goi/internal/events"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// windowBus delivers a window's events to that window only.
type windowBus struct {
	w application.Window
}

func newWindowBus(w application.Window) events.Bus { return windowBus{w: w} }

func (b windowBus) Emit(topic string, payload any) {
	b.w.DispatchWailsEvent(&application.CustomEvent{Name: topic, Data: payload})
}

// lazyWindow registers windows Goi didn't create; only server mode has them.
func lazyWindow(*App, application.Window) *window { return nil }
