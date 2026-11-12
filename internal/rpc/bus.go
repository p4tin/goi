package rpc

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// wailsBus publishes core events to the frontend through Wails events.
type wailsBus struct {
	ctx context.Context
}

func (b wailsBus) Emit(topic string, payload any) {
	runtime.EventsEmit(b.ctx, topic, payload)
}

// v2Window is the single Wails v2 window.
type v2Window struct {
	ctx context.Context
}

func (h v2Window) Focus() {
	runtime.WindowUnminimise(h.ctx)
	runtime.WindowShow(h.ctx)
}

func (h v2Window) SetTitle(title string) { runtime.WindowSetTitle(h.ctx, title) }
