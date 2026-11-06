package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/vfs"
)

func eventually(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met within", timeout)
}

type fixture struct {
	root string
	bus  *events.Recorder
	st   *settings.Store
	w    vfs.Watcher
	m    *Manager
}

func open(t *testing.T, files map[string]string) *fixture {
	t.Helper()
	root := t.TempDir()
	write(t, root, files)
	bus := &events.Recorder{}
	st, err := settings.Load(filepath.Join(t.TempDir(), "user.toml"), SettingsPath(root), bus)
	if err != nil {
		t.Fatal(err)
	}
	w, err := vfs.NewWatcher([]string{root}, vfs.MatchNone, bus)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	m, err := Open(context.Background(), Options{Root: root, Settings: st, Watcher: w, Bus: bus})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return &fixture{root, bus, st, w, m}
}

func TestManagerOpenAndRedetectOnGoModChange(t *testing.T) {
	f := open(t, map[string]string{"README.md": "x\n"})
	if k := f.m.Project().Kind; k != KindFolder {
		t.Fatalf("kind = %s", k)
	}
	if got := f.m.Project().Excludes; !slices.Contains(got, ".git") {
		t.Errorf("excludes from defaults = %v", got)
	}
	write(t, f.root, map[string]string{"go.mod": "module example.com/m\n\ngo 1.26\n"})
	eventually(t, 2*time.Second, func() bool { return f.m.Project().Kind == KindModule })

	n := len(f.bus.Events(events.TopicWorkspaceChanged))
	if n < 2 {
		t.Errorf("workspace.changed emitted %d times, want >= 2", n)
	}
}

func TestManagerParseErrorNotifiesOnce(t *testing.T) {
	f := open(t, map[string]string{"go.mod": "go 1.26\n"})
	if len(f.m.Project().ParseErrors) != 1 {
		t.Fatalf("errors = %+v", f.m.Project().ParseErrors)
	}
	notes := f.bus.Events(events.TopicNotify)
	if len(notes) != 1 {
		t.Fatalf("notify = %d, want 1", len(notes))
	}
	nt := notes[0].Payload.(events.Notification)
	if nt.Line != 1 || nt.File == "" {
		t.Errorf("notification = %+v", nt)
	}
}

func TestManagerContentRootsAndExcludes(t *testing.T) {
	f := open(t, map[string]string{"go.mod": "module example.com/m\n"})
	extra := t.TempDir()
	if err := f.m.AddContentRoot(extra); err != nil {
		t.Fatal(err)
	}
	eventually(t, 2*time.Second, func() bool { return slices.Contains(f.m.Project().ContentRoots, extra) })
	if got := f.st.Effective().Project.ContentRoots; !slices.Equal(got, []string{extra}) {
		t.Errorf("persisted roots = %v", got)
	}
	if err := f.m.AddContentRoot(filepath.Join(extra, "missing")); err == nil {
		t.Error("missing dir: want error")
	}
	if err := f.m.RemoveContentRoot(extra); err != nil {
		t.Fatal(err)
	}
	eventually(t, 2*time.Second, func() bool { return !slices.Contains(f.m.Project().ContentRoots, extra) })

	if err := f.m.SetExcludes([]string{"[bad"}); err == nil {
		t.Error("invalid glob: want error")
	}
	if err := f.m.SetExcludes([]string{"build"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, 2*time.Second, func() bool { return slices.Equal(f.m.Project().Excludes, []string{"build"}) })
	if !f.m.Matcher().Match("build", true) {
		t.Error("matcher not updated")
	}
	if _, err := os.Stat(SettingsPath(f.root)); err != nil {
		t.Errorf("project settings not written: %v", err)
	}
}

func TestManagerCloseStopsUpdates(t *testing.T) {
	f := open(t, map[string]string{"README.md": "x\n"})
	f.m.Close()
	write(t, f.root, map[string]string{"go.mod": "module example.com/m\n"})
	time.Sleep(600 * time.Millisecond)
	if f.m.Project().Kind != KindFolder {
		t.Error("project changed after Close")
	}
}
