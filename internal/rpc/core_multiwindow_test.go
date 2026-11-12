package rpc

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/vfs"
)

func TestInfoAndProjectAreTheCallingWindows(t *testing.T) {
	app, wA, wB, _, _ := twoWindows(t)
	sA, _ := wA.current()
	sB, _ := wB.current()
	ctxA := withWindow(context.Background(), wA)
	ctxB := withWindow(context.Background(), wB)
	if got := app.appSvc.Info(ctxB).ProjectPath; got != sB.root {
		t.Errorf("Info from B: ProjectPath = %q, want %q", got, sB.root)
	}
	if got := app.appSvc.Info(ctxA).ProjectPath; got != sA.root {
		t.Errorf("Info from A: ProjectPath = %q, want %q", got, sA.root)
	}
	if p := app.workspaceSvc.Project(ctxB); p == nil || p.Root != sB.root {
		t.Errorf("Project from B = %+v, want root %s", p, sB.root)
	}
}

func TestFileGuardUsesTheCallingWindowsProject(t *testing.T) {
	app, wA, wB, _, _ := twoWindows(t)
	sA, _ := wA.current()
	ctxA := withWindow(context.Background(), wA)
	ctxB := withWindow(context.Background(), wB)
	fs := app.fileSvc
	if _, err := fs.List(ctxA, sA.root); err != nil {
		t.Errorf("List A's root from A: %v", err)
	}
	if _, err := fs.List(ctxB, sA.root); err == nil {
		t.Error("List A's root from B: want an outside-the-project error")
	}
	path := filepath.Join(sA.root, "new.txt")
	if err := fs.Create(ctxB, path, false, "x"); err == nil {
		t.Error("Create in A's project from B: want an outside-the-project error")
	}
	if err := fs.Create(ctxA, path, false, "x"); err != nil {
		t.Errorf("Create in A's project from A: %v", err)
	}
	if _, err := fs.Write(ctxB, path, "y", vfs.FileMeta{}); err == nil {
		t.Error("Write in A's project from B: want an outside-the-project error")
	}
}

func TestProjectSettingsArePerWindow(t *testing.T) {
	app, wA, wB, _, _ := twoWindows(t)
	ctxA := withWindow(context.Background(), wA)
	ctxB := withWindow(context.Background(), wB)
	svc := app.settingsSvc
	before, err := svc.Effective(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Set(ctxB, "project", "editor.tabSize", 9); err != nil {
		t.Fatal(err)
	}
	if b, _ := svc.Effective(ctxB); b.Editor.TabSize != 9 {
		t.Errorf("B tabSize = %d, want 9", b.Editor.TabSize)
	}
	if a, _ := svc.Effective(ctxA); a.Editor.TabSize != before.Editor.TabSize {
		t.Errorf("A tabSize = %d after B's project write, want %d", a.Editor.TabSize, before.Editor.TabSize)
	}
	if o, _ := svc.Origins(ctxA); o["editor.tabSize"] == "project" {
		t.Error("A's tabSize comes from the project layer after B's write")
	}
	pA, _ := svc.Paths(ctxA)
	pB, _ := svc.Paths(ctxB)
	if pA.Project == pB.Project || pA.User != pB.User {
		t.Errorf("paths A = %+v, B = %+v", pA, pB)
	}
}

func TestOpenAndCloseProjectActOnTheCallingWindow(t *testing.T) {
	app, _ := started(t, "")
	welcome := app.allWindows()[0]
	other, err := app.addWindow(nil, &events.Recorder{})
	if err != nil {
		t.Fatal(err)
	}
	rootB := module(t)
	if err := other.openProject(rootB); err != nil {
		t.Fatal(err)
	}
	withWindows(app) // a new window would be recorded, not native

	root := module(t)
	if err := app.appSvc.OpenProject(withWindow(context.Background(), welcome), root, false); err != nil {
		t.Fatal(err)
	}
	if got := app.windowWithProject(root); got != welcome || len(app.allWindows()) != 2 {
		t.Fatalf("project opened in window %v (windows = %d), want the Welcome window", got, len(app.allWindows()))
	}

	if err := app.appSvc.CloseProject(withWindow(context.Background(), welcome)); err != nil {
		t.Fatal(err)
	}
	if _, err := welcome.current(); err == nil {
		t.Error("calling window still has a project")
	}
	if s, err := other.current(); err != nil || s.root != rootB {
		t.Errorf("other window lost its project: %v", err)
	}
}
