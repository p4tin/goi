package rpc

import (
	"context"
	"testing"
	"time"
)

func TestBeforeCloseHandshake(t *testing.T) {
	a, bus := started(t, "")
	now := time.Unix(1000, 0)
	a.mainWindow().closing.now = func() time.Time { return now }
	quits := 0
	a.mainWindow().closing.quitFn = func(context.Context) { quits++ }

	if !a.BeforeClose(context.Background()) {
		t.Fatal("first close request should be prevented")
	}
	if n := len(bus.Events(TopicBeforeClose)); n != 1 {
		t.Fatalf("beforeClose events = %d, want 1", n)
	}

	a.appSvc.ConfirmClose()
	if quits != 1 {
		t.Errorf("quit calls = %d, want 1", quits)
	}
	if a.BeforeClose(context.Background()) {
		t.Error("close after ConfirmClose should be allowed")
	}
}

func TestBeforeCloseRepeatedRequestForcesClose(t *testing.T) {
	a, bus := started(t, "")
	now := time.Unix(1000, 0)
	a.mainWindow().closing.now = func() time.Time { return now }

	if !a.BeforeClose(context.Background()) {
		t.Fatal("first request should be prevented")
	}
	now = now.Add(5 * time.Second)
	if !a.BeforeClose(context.Background()) {
		t.Fatal("request after the window should be prevented again")
	}
	now = now.Add(time.Second)
	if a.BeforeClose(context.Background()) {
		t.Error("second request within 2s should be allowed")
	}
	if n := len(bus.Events(TopicBeforeClose)); n != 2 {
		t.Errorf("beforeClose events = %d, want 2", n)
	}
}
