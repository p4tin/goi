package rpc

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/p4tin/goi/internal/events"
)

// withWindows lets a started app open more windows, each with a fake handle
// and its own recorder. It returns the recorders of the windows opened.
func withWindows(a *App) *[]*events.Recorder {
	var recs []*events.Recorder
	a.makeWindow = func() (windowHandle, events.Bus) {
		r := &events.Recorder{}
		recs = append(recs, r)
		return &fakeHandle{}, r
	}
	return &recs
}

func TestOpenProjectFocusesWindowThatHasIt(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	var focused atomic.Int32
	a.mainWindow().handle = &fakeHandle{focus: func() { focused.Add(1) }}
	recs := withWindows(a)
	if err := a.appSvc.OpenProject(root, true); err != nil {
		t.Fatal(err)
	}
	if focused.Load() != 1 || len(*recs) != 0 {
		t.Errorf("focused = %d, new windows = %d", focused.Load(), len(*recs))
	}
}

func TestOpenProjectWindowRules(t *testing.T) {
	root := module(t)
	a, _ := started(t, "")
	recs := withWindows(a)

	// newWindow: a new window even though this one shows Welcome.
	if err := a.appSvc.OpenProject(root, true); err != nil {
		t.Fatal(err)
	}
	if len(*recs) != 1 || len(a.allWindows()) != 2 {
		t.Fatalf("new windows = %d, windows = %d", len(*recs), len(a.allWindows()))
	}
	if a.windowWithProject(root) == a.allWindows()[0] {
		t.Error("project opened in the Welcome window")
	}

	// The same project again: focuses its window, no third window.
	if err := a.openProjectWindow(root, a.allWindows()[0]); err != nil {
		t.Fatal(err)
	}
	if len(a.allWindows()) != 2 {
		t.Fatalf("windows = %d after reopening the same project", len(a.allWindows()))
	}

	// Another project from the Welcome window: opens there.
	other := module(t)
	first := a.allWindows()[0]
	if err := a.openProjectWindow(other, first); err != nil {
		t.Fatal(err)
	}
	if a.windowWithProject(other) != first || len(a.allWindows()) != 2 {
		t.Errorf("Welcome window not reused")
	}

	// A third project from a window that has one: a new window.
	third := module(t)
	if err := a.openProjectWindow(third, first); err != nil {
		t.Fatal(err)
	}
	if w := a.windowWithProject(third); w == nil || w == first || len(a.allWindows()) != 3 {
		t.Errorf("third project not in a new window")
	}
}

func TestWindowWithProjectFollowsSymlinks(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	withWindows(a)
	if err := a.openProjectWindow(link, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(a.allWindows()); n != 1 {
		t.Errorf("windows = %d; the symlinked path opened a second copy", n)
	}
}

func TestSecondInstanceOpensRelativeProject(t *testing.T) {
	a, _ := started(t, "")
	root := module(t)
	withWindows(a)
	a.secondInstance([]string{"/Applications/Goi.app/Contents/MacOS/goi", "--project", filepath.Base(root)}, filepath.Dir(root))
	if w := a.windowWithProject(root); w == nil {
		t.Fatalf("project %s not opened", root)
	} else if w != a.allWindows()[0] {
		t.Error("project should open in the Welcome window")
	}
}

func TestSecondInstanceOpensFileInItsWindow(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	withWindows(a)
	file := filepath.Join(root, "pkg", "util", "util.go")
	a.secondInstance([]string{"goi", file}, "/")
	w := a.windowWithProject(root)
	evs := w.bus.(*events.Recorder).Events(TopicOpenFile)
	if len(evs) != 1 || evs[0].Payload.(OpenFileEvent).Path != file {
		t.Errorf("openFile events = %+v", evs)
	}
	if len(a.allWindows()) != 1 {
		t.Errorf("windows = %d", len(a.allWindows()))
	}
}

func TestSecondInstanceWithoutArgsFocuses(t *testing.T) {
	a, _ := started(t, "")
	var focused atomic.Int32
	a.mainWindow().handle = &fakeHandle{focus: func() { focused.Add(1) }}
	a.secondInstance([]string{"goi"}, "/")
	if focused.Load() != 1 {
		t.Errorf("focused = %d", focused.Load())
	}
}

// closingHandle is a fake native window whose Close runs the close hook like
// Wails does: the window closes unless the hook prevents it.
type closingHandle struct {
	fakeHandle
	w      *window
	closed int
}

func (h *closingHandle) Close() {
	if h.w.beforeClose() {
		return
	}
	h.closed++
	h.w.app.windowClosed(h.w)
}

func TestQuitClosesWindowsAndStopsOnCancel(t *testing.T) {
	a, _ := started(t, module(t))
	withWindows(a)
	if err := a.openWindow(module(t)); err != nil {
		t.Fatal(err)
	}
	ws := a.allWindows()
	hs := make([]*closingHandle, len(ws))
	for i, w := range ws {
		hs[i] = &closingHandle{w: w}
		w.handle = hs[i]
	}
	quits := 0
	a.quitFn = func() { quits++ }
	ctx := func(w *window) context.Context { return withWindow(context.Background(), w) }

	// Quit asks the focused (newest) window first.
	if a.shouldQuit() {
		t.Fatal("quit with open windows went straight through")
	}
	if n := len(ws[1].bus.(*events.Recorder).Events(TopicBeforeClose)); n != 1 {
		t.Fatalf("second window beforeClose = %d", n)
	}
	// The user cancels: the quit stops and both windows stay.
	a.appSvc.CancelClose(ctx(ws[1]))
	if a.quitRequested.Load() || len(a.allWindows()) != 2 {
		t.Fatalf("quit still pending or a window closed")
	}

	// Quit again and confirm both windows.
	a.shouldQuit()
	a.appSvc.ConfirmClose(ctx(ws[1]))
	if hs[1].closed != 1 || len(a.allWindows()) != 1 {
		t.Fatalf("second window not closed")
	}
	if n := len(ws[0].bus.(*events.Recorder).Events(TopicBeforeClose)); n != 1 {
		t.Fatalf("first window not asked after the second closed (beforeClose = %d)", n)
	}
	a.appSvc.ConfirmClose(ctx(ws[0]))
	if len(a.allWindows()) != 0 || quits != 1 || !a.shouldQuit() {
		t.Errorf("windows = %d, quits = %d", len(a.allWindows()), quits)
	}
	if got := a.recent.OpenSet(); len(got) != 2 {
		t.Errorf("open set = %v, want both projects", got)
	}
}

func TestStartupReopensOpenSet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOI_HOME", home)
	rootA, rootB := module(t), module(t)
	a, err := NewApp(Options{})
	if err != nil {
		t.Fatal(err)
	}
	withWindows(a)
	// Seed the open set the way a previous quit would.
	pre, _ := NewApp(Options{})
	pre.start(context.Background(), events.Nop{}, nil)
	pre.recent.SetOpenSet([]string{rootA, rootB, filepath.Join(home, "gone")})
	pre.Shutdown(context.Background())

	if err := a.start(context.Background(), &events.Recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	defer a.Shutdown(context.Background())
	ws := a.allWindows()
	if len(ws) != 2 || a.windowWithProject(rootA) != ws[0] || a.windowWithProject(rootB) != ws[1] {
		t.Fatalf("windows = %d; want A in the first and B in a second", len(ws))
	}
	for _, w := range ws {
		if !w.ready.Load() {
			t.Errorf("window %d not ready", w.id)
		}
	}
}

func TestWindowIDWaitsForReady(t *testing.T) {
	a, _ := started(t, "")
	w := a.mainWindow()
	ctx := withWindow(context.Background(), w)
	if id := a.appSvc.WindowID(ctx); id != w.id || id == 0 {
		t.Fatalf("WindowID = %d, want %d", id, w.id)
	}
	w.ready.Store(false)
	if id := a.appSvc.WindowID(ctx); id != 0 {
		t.Errorf("WindowID before ready = %d, want 0", id)
	}
}
