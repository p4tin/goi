package rpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/workspace"
)

// twoWindows starts an app with project a in the first window and project b
// in a second one, each with its own recorder.
func twoWindows(t *testing.T) (app *App, w1, w2 *window, r1, r2 *events.Recorder) {
	t.Helper()
	rootA, rootB := module(t), module(t)
	app, r1 = started(t, rootA)
	w1 = app.mainWindow()
	r2 = &events.Recorder{}
	w2, err := app.addWindow(nil, r2)
	if err != nil {
		t.Fatal(err)
	}
	if err := w2.openProject(rootB); err != nil {
		t.Fatal(err)
	}
	return app, w1, w2, r1, r2
}

func TestWindowForRoutesByContext(t *testing.T) {
	app, w1, w2, _, _ := twoWindows(t)
	s1, err := app.sessionFor(withWindow(context.Background(), w1))
	if err != nil {
		t.Fatal(err)
	}
	s2, err := app.sessionFor(withWindow(context.Background(), w2))
	if err != nil {
		t.Fatal(err)
	}
	if s1.root == s2.root || s1.win != w1 || s2.win != w2 {
		t.Fatalf("sessions not per window: %s (%p) / %s (%p)", s1.root, s1.win, s2.root, s2.win)
	}
	if _, err := app.windowFor(context.Background()); !errors.Is(err, errNoWindow) {
		t.Errorf("windowFor without a window and two windows: err = %v, want errNoWindow", err)
	}
}

func TestProjectEventsStayInTheirWindow(t *testing.T) {
	_, w1, w2, r1, r2 := twoWindows(t)
	s1, _ := w1.current()
	s2, _ := w2.current()
	for _, c := range []struct {
		rec  *events.Recorder
		want string
	}{{r1, s1.root}, {r2, s2.root}} {
		evs := c.rec.Events(events.TopicWorkspaceChanged)
		if len(evs) == 0 {
			t.Fatalf("no workspace.changed for %s", c.want)
		}
		for _, ev := range evs {
			p, ok := ev.Payload.(workspace.Project)
			if !ok {
				t.Fatalf("workspace.changed payload is %T", ev.Payload)
			}
			if p.Root != c.want {
				t.Errorf("window for %s got workspace.changed for %s", c.want, p.Root)
			}
		}
	}

	// Closing one window's project leaves the other open.
	if err := w1.closeProject(); err != nil {
		t.Fatal(err)
	}
	if _, err := w2.current(); err != nil {
		t.Errorf("second window lost its project: %v", err)
	}
}

func TestUserSettingsReachEveryWindow(t *testing.T) {
	_, w1, w2, _, _ := twoWindows(t)
	if err := w1.settings.Set(settings.LayerUser, "editor.tabSize", 7); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for w2.settings.Effective().Editor.TabSize != 7 {
		if time.Now().After(deadline) {
			t.Fatalf("second window tabSize = %d, want 7", w2.settings.Effective().Editor.TabSize)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRemoveWindowClosesItsSession(t *testing.T) {
	app, w1, w2, _, _ := twoWindows(t)
	app.removeWindow(w2)
	if got := app.allWindows(); len(got) != 1 || got[0] != w1 {
		t.Fatalf("windows = %v", got)
	}
	if _, err := w2.current(); !errors.Is(err, errNoProject) {
		t.Errorf("removed window still has a session")
	}
	if _, err := w1.current(); err != nil {
		t.Errorf("first window lost its project: %v", err)
	}
}
