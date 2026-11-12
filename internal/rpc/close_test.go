package rpc

import (
	"context"
	"testing"
	"time"
)

func TestBeforeCloseHandshake(t *testing.T) {
	a, bus := started(t, "")
	w := a.mainWindow()
	now := time.Unix(1000, 0)
	w.closing.now = func() time.Time { return now }
	closes := 0
	w.handle = &fakeHandle{}
	a.quitFn = func() {}

	if !w.beforeClose() {
		t.Fatal("first close request should be prevented")
	}
	if n := len(bus.Events(TopicBeforeClose)); n != 1 {
		t.Fatalf("beforeClose events = %d, want 1", n)
	}

	h := &closingHandle{w: w}
	w.handle = h
	a.appSvc.ConfirmClose(withWindow(context.Background(), w))
	closes = h.closed
	if closes != 1 {
		t.Errorf("window closes = %d, want 1", closes)
	}
	if w.beforeClose() {
		t.Error("close after ConfirmClose should be allowed")
	}
}

func TestBeforeCloseRepeatedRequestForcesClose(t *testing.T) {
	a, bus := started(t, "")
	w := a.mainWindow()
	now := time.Unix(1000, 0)
	w.closing.now = func() time.Time { return now }

	if !w.beforeClose() {
		t.Fatal("first request should be prevented")
	}
	now = now.Add(5 * time.Second)
	if !w.beforeClose() {
		t.Fatal("a request long after the first should be prevented again")
	}
	now = now.Add(time.Second)
	if w.beforeClose() {
		t.Error("a repeated request within the force window should be allowed")
	}
	if n := len(bus.Events(TopicBeforeClose)); n != 2 {
		t.Errorf("beforeClose events = %d, want 2", n)
	}
}

func TestCancelCloseResetsForceWindow(t *testing.T) {
	a, _ := started(t, "")
	w := a.mainWindow()
	now := time.Unix(1000, 0)
	w.closing.now = func() time.Time { return now }
	w.beforeClose()
	a.appSvc.CancelClose(withWindow(context.Background(), w))
	now = now.Add(time.Second)
	if !w.beforeClose() {
		t.Error("a close right after a cancel must ask again, not force")
	}
}

func TestLastWindowQuitsOffMac(t *testing.T) {
	a, _ := started(t, "")
	quits := 0
	a.quitFn = func() { quits++ }
	w := a.mainWindow()
	w.handle = &closingHandle{w: w}
	w.requestClose()
	a.appSvc.ConfirmClose(withWindow(context.Background(), w))
	want := 1
	if isMac() {
		want = 0 // Mac apps keep running without windows
	}
	if quits != want || len(a.allWindows()) != 0 {
		t.Errorf("quits = %d (want %d), windows = %d", quits, want, len(a.allWindows()))
	}
}
