package rpc

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// TopicBeforeClose asks the frontend to resolve unsaved work before the
// window closes; it answers with AppService.ConfirmClose.
const TopicBeforeClose = "app.beforeClose"

// forceCloseWindow: a second close request this soon after an unanswered
// first one is allowed, so a hung frontend can never trap the window.
const forceCloseWindow = 2 * time.Second

// closeState tracks the close handshake with the frontend.
type closeState struct {
	allow atomic.Bool

	mu          sync.Mutex
	lastRequest time.Time

	// quitFn quits the application; tests replace it.
	quitFn func(ctx context.Context)
	now    func() time.Time
}

// BeforeClose is the Wails OnBeforeClose hook. It prevents the close and asks
// the frontend to confirm, unless the close was already confirmed or this is
// a repeated request within forceCloseWindow.
func (a *App) BeforeClose(ctx context.Context) (prevent bool) {
	c := &a.closing
	if c.allow.Load() {
		return false
	}
	now := c.clock()
	c.mu.Lock()
	repeated := !c.lastRequest.IsZero() && now.Sub(c.lastRequest) < forceCloseWindow
	c.lastRequest = now
	c.mu.Unlock()
	if repeated {
		a.log.Warn("close forced: frontend did not confirm")
		c.allow.Store(true)
		return false
	}
	a.bus.Emit(TopicBeforeClose, nil)
	return true
}

func (c *closeState) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// ConfirmClose is called by the frontend once unsaved work is resolved; it
// allows the pending close and quits.
func (s *AppService) ConfirmClose() {
	c := &s.app.closing
	c.allow.Store(true)
	if c.quitFn != nil {
		c.quitFn(s.app.ctx)
		return
	}
	wruntime.Quit(s.app.ctx)
}
