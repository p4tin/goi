package rpc

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/instance"
	"github.com/p4tin/goi/internal/workspace"
)

// holdLockAsOtherProcess acquires root's instance lock and rewrites its pid
// to the (live) parent process, so this process sees it as another window.
func holdLockAsOtherProcess(t *testing.T, cacheDir, root string, onFocus func()) func() {
	t.Helper()
	hash := workspace.Hash(root)
	l, err := instance.Acquire(cacheDir, hash, onFocus)
	if err != nil {
		t.Fatal(err)
	}
	path := instance.LockPath(cacheDir, hash)
	b, _ := os.ReadFile(path)
	f := strings.Fields(string(b))
	os.WriteFile(path, []byte(fmt.Sprintf("%d %s %s\n", os.Getppid(), f[1], f[2])), 0o600)
	return func() {
		l.Release()
		os.Remove(path)
	}
}

func TestOpenInOtherWindowFocusesSelfForSameProject(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	var focused atomic.Int32
	a.mainWindow().handle = &fakeHandle{focus: func() { focused.Add(1) }}
	if err := a.appSvc.OpenProject(root, true); err != nil {
		t.Fatal(err)
	}
	if focused.Load() != 1 {
		t.Errorf("focused = %d, want 1", focused.Load())
	}
}

func TestOpenProjectNewWindowLaunches(t *testing.T) {
	root := module(t)
	a, _ := started(t, "")
	var launched []string
	orig := launchNewWindow
	launchNewWindow = func(p string) error { launched = append(launched, p); return nil }
	defer func() { launchNewWindow = orig }()

	if err := a.appSvc.OpenProject(root, true); err != nil {
		t.Fatal(err)
	}
	if len(launched) != 1 || launched[0] != root {
		t.Errorf("launched = %v", launched)
	}
	if a.workspaceSvc.Project() != nil {
		t.Error("newWindow should not open in this window")
	}

	// No project here and !newWindow: opens in this window.
	if err := a.appSvc.OpenProject(root, false); err != nil {
		t.Fatal(err)
	}
	if p := a.workspaceSvc.Project(); p == nil || p.Root != root {
		t.Fatalf("project = %+v", p)
	}
	// A different project while one is open: launches instead of failing.
	other := module(t)
	if err := a.appSvc.OpenProject(other, false); err != nil {
		t.Fatal(err)
	}
	if len(launched) != 2 || launched[1] != other {
		t.Errorf("launched = %v", launched)
	}
}

func TestOpenElsewhereWhenLockHeldByOtherProcess(t *testing.T) {
	root := module(t)
	a, _ := started(t, "")
	var focused atomic.Int32
	stop := holdLockAsOtherProcess(t, a.cacheDir, root, func() { focused.Add(1) })
	defer stop()

	err := a.openProject(root)
	if !errors.Is(err, ErrOpenElsewhere) {
		t.Fatalf("err = %v, want ErrOpenElsewhere", err)
	}
	if focused.Load() != 1 {
		t.Errorf("holder focused = %d, want 1", focused.Load())
	}
	if a.workspaceSvc.Project() != nil {
		t.Error("project opened despite lock")
	}

	// openInOtherWindow focuses the holder instead of launching.
	launches := 0
	orig := launchNewWindow
	launchNewWindow = func(string) error { launches++; return nil }
	defer func() { launchNewWindow = orig }()
	if err := a.appSvc.OpenProject(root, true); err != nil {
		t.Fatal(err)
	}
	if focused.Load() != 2 || launches != 0 {
		t.Errorf("focused = %d launches = %d", focused.Load(), launches)
	}
}

func TestStartupWithHeldProjectQuits(t *testing.T) {
	root := module(t)
	t.Setenv("GOI_HOME", t.TempDir())
	a, err := NewApp(Options{ProjectPath: root})
	if err != nil {
		t.Fatal(err)
	}
	var focused atomic.Int32
	stop := holdLockAsOtherProcess(t, a.cacheDir, root, func() { focused.Add(1) })
	defer stop()
	if err := a.start(t.Context(), events.Nop{}, nil); err != nil {
		t.Fatal(err)
	}
	defer a.Shutdown(t.Context())
	if !a.quitAfterStart || focused.Load() != 1 {
		t.Errorf("quitAfterStart = %v, focused = %d", a.quitAfterStart, focused.Load())
	}
}

func TestReleaseOnCloseAllowsReopen(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	if err := a.appSvc.CloseProject(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(instance.LockPath(a.cacheDir, workspace.Hash(root))); !os.IsNotExist(err) {
		t.Errorf("lock not released: %v", err)
	}
	if err := a.appSvc.OpenProject(root, false); err != nil {
		t.Fatal(err)
	}
}
