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
