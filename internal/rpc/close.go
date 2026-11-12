package rpc

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// TopicBeforeClose asks the frontend to resolve unsaved work before the
// window closes; it answers with AppService.ConfirmClose or CancelClose.
const TopicBeforeClose = "app.beforeClose"

// forceCloseWindow: a second close request this soon after an unanswered
// first one is allowed, so a hung frontend can never trap the window.
const forceCloseWindow = 2 * time.Second

// closeState tracks a window's close handshake with its frontend.
type closeState struct {
	allow atomic.Bool

	mu          sync.Mutex
	lastRequest time.Time

	now func() time.Time
}

// beforeClose is the window's native close hook. It prevents the close and
// asks the frontend to confirm, unless the close was already confirmed or
// this is a repeated request within forceCloseWindow.
func (w *window) beforeClose() (prevent bool) {
	c := &w.closing
	if c.allow.Load() {
		return false
	}
	now := c.clock()
	c.mu.Lock()
	repeated := !c.lastRequest.IsZero() && now.Sub(c.lastRequest) < forceCloseWindow
	c.lastRequest = now
	c.mu.Unlock()
	if repeated {
		w.app.log.Warn("close forced: frontend did not confirm")
		c.allow.Store(true)
		return false
	}
	w.bus.Emit(TopicBeforeClose, nil)
	return true
}

func (c *closeState) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// confirmClose allows the pending close and closes the window.
func (w *window) confirmClose() {
	w.closing.allow.Store(true)
	if w.handle != nil {
		w.handle.Close() // the close hook now lets it through
		return
	}
	w.app.windowClosed(w)
}

// ConfirmClose is called by the frontend once unsaved work is resolved; it
// closes the calling window.
func (s *AppService) ConfirmClose(ctx context.Context) {
	if w, err := s.app.windowFor(ctx); err == nil {
		w.confirmClose()
	}
}

// CancelClose is called by the frontend when the user keeps the window open;
// a quit in progress stops.
func (s *AppService) CancelClose(ctx context.Context) {
	if w, err := s.app.windowFor(ctx); err == nil {
		w.closing.mu.Lock()
		w.closing.lastRequest = time.Time{}
		w.closing.mu.Unlock()
	}
	s.app.cancelQuit()
}
