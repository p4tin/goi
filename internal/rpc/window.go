package rpc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/p4tin/goi/internal/instance"
	"github.com/p4tin/goi/internal/workspace"
)

// ErrOpenElsewhere means the project is open in another Goi window, which
// has been asked to come to the front.
var ErrOpenElsewhere = errors.New("project is already open in another Goi window")

// launchNewWindow starts a new Goi process; tests replace it.
var launchNewWindow = instance.LaunchNewWindow

// acquireInstance takes the per-project instance lock. If another live
// window holds it, that window is focused and ErrOpenElsewhere is returned.
func (a *App) acquireInstance(root string) (*instance.Lock, error) {
	hash := workspace.Hash(root)
	lock, err := instance.Acquire(a.cacheDir, hash, func() {
		if a.focusFn != nil {
			a.focusFn()
		}
	})
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
func (a *App) openInOtherWindow(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("open project: %w", err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return fmt.Errorf("open project: %s is not a folder", abs)
	}
	if sess, err := a.current(); err == nil && sess.root == abs {
		if a.focusFn != nil {
			a.focusFn()
		}
		return nil
	}
	hash := workspace.Hash(abs)
	if instance.Held(a.cacheDir, hash) {
		return instance.Focus(a.cacheDir, hash)
	}
	a.log.Info("launch new window", "project", abs)
	return launchNewWindow(abs)
}
