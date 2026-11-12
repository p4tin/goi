package rpc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/vfs"
	"github.com/p4tin/goi/internal/workspace"
)

func TestParseArgs(t *testing.T) {
	abs, _ := filepath.Abs("some/dir")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"none", nil, ""},
		{"space form", []string{"--project", "some/dir"}, abs},
		{"equals form", []string{"--project=some/dir"}, abs},
		{"single dash", []string{"-project", "some/dir"}, abs},
		{"unknown flags ignored", []string{"-psn_0_12345", "--project", "some/dir", "--weird"}, abs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseArgs(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if got.ProjectPath != tt.want {
				t.Errorf("ProjectPath = %q, want %q", got.ProjectPath, tt.want)
			}
		})
	}
}

// started returns an App started with a Recorder bus and isolated GOI_HOME.
func started(t *testing.T, project string) (*App, *events.Recorder) {
	t.Helper()
	t.Setenv("GOI_HOME", t.TempDir())
	a, err := NewApp(Options{ProjectPath: project})
	if err != nil {
		t.Fatal(err)
	}
	bus := &events.Recorder{}
	if err := a.start(context.Background(), bus, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Shutdown(context.Background()) })
	return a, bus
}

func module(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.26\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "pkg", "util"), 0o755)
	os.WriteFile(filepath.Join(root, "pkg", "util", "util.go"), []byte("package helpers\n"), 0o644)
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	return root
}

func TestStartupOpensProjectAndRecordsRecent(t *testing.T) {
	root := module(t)
	a, bus := started(t, root)

	p := a.workspaceSvc.Project(context.Background())
	if p == nil || p.Kind != workspace.KindModule || p.Root != root {
		t.Fatalf("project = %+v", p)
	}
	if info := a.appSvc.Info(context.Background()); info.ProjectPath != root || info.Name != "Goi" {
		t.Errorf("info = %+v", info)
	}
	list, err := a.recentSvc.List()
	if err != nil || len(list) != 1 || list[0].Path != root || list[0].Kind != "module" {
		t.Errorf("recent = %+v, %v", list, err)
	}
	if len(bus.Events(events.TopicWorkspaceChanged)) == 0 {
		t.Error("no workspace.changed event")
	}
	// A second project goes to a new window, never this one.
	recs := withWindows(a)
	other := t.TempDir()
	if err := a.appSvc.OpenProject(context.Background(), other, false); err != nil || len(*recs) != 1 || a.windowWithProject(other) == a.allWindows()[0] {
		t.Errorf("second project: err=%v new windows=%d", err, len(*recs))
	}
	if err := a.allWindows()[0].openProject(other); err == nil {
		t.Error("openProject with a project already open: want error")
	}
}

func TestReopenLastProjectAndClose(t *testing.T) {
	root := module(t)
	home := t.TempDir()
	t.Setenv("GOI_HOME", home)
	a1, _ := NewApp(Options{ProjectPath: root})
	a1.start(context.Background(), events.Nop{}, nil)
	a1.Shutdown(context.Background())

	a2, err := NewApp(Options{})
	if err != nil {
		t.Fatal(err)
	}
	a2.start(context.Background(), events.Nop{}, nil)
	defer a2.Shutdown(context.Background())
	if p := a2.workspaceSvc.Project(context.Background()); p == nil || p.Root != root {
		t.Fatalf("reopened project = %+v", p)
	}
	if err := a2.appSvc.CloseProject(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a2.workspaceSvc.Project(context.Background()) != nil {
		t.Error("project still open after CloseProject")
	}
}

func TestFileServiceGuardAndOps(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	fs := a.fileSvc
	ctx := context.Background()

	entries, err := fs.List(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == ".git" {
			t.Error(".git should be excluded")
		}
	}
	if _, err := fs.List(ctx, t.TempDir()); err == nil {
		t.Error("List outside project: want error")
	}

	outside := filepath.Join(t.TempDir(), "x.txt")
	if err := fs.Create(ctx, outside, false, "x"); err == nil {
		t.Error("Create outside project: want error")
	}
	if err := fs.Trash(ctx, root); err == nil {
		t.Error("Trash content root: want error")
	}
	if _, err := fs.Write(ctx, filepath.Join(root, "..", "escape.txt"), "x", vfs.FileMeta{}); err == nil {
		t.Error("Write via .. escape: want error")
	}

	p, err := fs.CreateGoFile(ctx, filepath.Join(root, "pkg", "util"), "more")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "package helpers\n" {
		t.Errorf("CreateGoFile content = %q", b)
	}
	newDir := filepath.Join(root, "my-tool")
	if err := fs.Create(ctx, newDir, true, ""); err != nil {
		t.Fatal(err)
	}
	p2, _ := fs.CreateGoFile(ctx, newDir, "main.go")
	b2, _ := os.ReadFile(p2)
	if string(b2) != "package mytool\n" {
		t.Errorf("derived package = %q", b2)
	}

	f, err := fs.Read(ctx, filepath.Join(root, "go.mod"))
	if err != nil || !strings.HasPrefix(f.Content, "module example.com/m") || f.Meta.Mode != vfs.ModeNormal {
		t.Errorf("Read = %+v, %v", f, err)
	}
	if _, err := fs.Write(ctx, f.Meta.Path, f.Content+"\n// x\n", f.Meta); err != nil {
		t.Errorf("Write inside project: %v", err)
	}
}

func TestSettingsAndLayout(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	if err := a.settingsSvc.Set(context.Background(), "project", "save.format", "gofmt"); err != nil {
		t.Fatal(err)
	}
	if eff, err := a.settingsSvc.Effective(context.Background()); err != nil || eff.Save.Format != "gofmt" {
		t.Errorf("format = %q, %v", eff.Save.Format, err)
	}
	if o, err := a.settingsSvc.Origins(context.Background()); err != nil || o["save.format"] != "project" {
		t.Errorf("origin = %q, %v", o["save.format"], err)
	}
	if err := a.settingsSvc.Set(context.Background(), "default", "save.format", "none"); err == nil {
		t.Error("write default layer: want error")
	}
	if _, err := os.Stat(filepath.Join(root, ".goi", "settings.toml")); err != nil {
		t.Errorf("project settings file: %v", err)
	}

	if s, err := a.appSvc.LoadLayout(context.Background()); err != nil || s != "" {
		t.Errorf("initial layout = %q, %v", s, err)
	}
	if err := a.appSvc.SaveLayout(context.Background(), `{"left":240}`); err != nil {
		t.Fatal(err)
	}
	if s, _ := a.appSvc.LoadLayout(context.Background()); s != `{"left":240}` {
		t.Errorf("layout = %q", s)
	}
}

func TestSanitizePackage(t *testing.T) {
	for in, want := range map[string]string{"my-tool": "mytool", "Foo.Bar": "foobar", "2fa": "p2fa", "---": "main", "util": "util"} {
		if got := sanitizePackage(in); got != want {
			t.Errorf("sanitizePackage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearchServiceIndexesAndFollowsChanges(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	var res FileSearchResult
	eventuallyRPC(t, func() bool {
		res, _ = a.searchSvc.Files(context.Background(), "util", 10, nil)
		return res.Index.Ready && len(res.Matches) > 0
	})
	if res.Matches[0].Rel != "pkg/util/util.go" {
		t.Errorf("top = %+v", res.Matches[0])
	}
	for _, m := range res.Matches {
		if strings.HasPrefix(m.Rel, ".git/") {
			t.Errorf("excluded path indexed: %s", m.Rel)
		}
	}
	os.WriteFile(filepath.Join(root, "brandnew.go"), []byte("package m\n"), 0o644)
	eventuallyRPC(t, func() bool {
		res, _ = a.searchSvc.Files(context.Background(), "brandnew", 10, nil)
		return len(res.Matches) == 1
	})
	os.Remove(filepath.Join(root, "brandnew.go"))
	eventuallyRPC(t, func() bool {
		res, _ = a.searchSvc.Files(context.Background(), "brandnew", 10, nil)
		return len(res.Matches) == 0
	})
}

func eventuallyRPC(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 150; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met within 3s")
}

func TestJournalRestoreAfterCrash(t *testing.T) {
	root := module(t)
	home := t.TempDir()
	t.Setenv("GOI_HOME", home)
	a1, err := NewApp(Options{ProjectPath: root})
	if err != nil {
		t.Fatal(err)
	}
	a1.start(context.Background(), events.Nop{}, nil)
	file := filepath.Join(root, "go.mod")
	if err := a1.appSvc.JournalPut(context.Background(), file, "module changed\n", time.Now().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash: pretend the journal's owner died by rewriting
	// session.lock with a dead pid.
	w1 := a1.mainWindow()
	w1.mu.Lock()
	sess := w1.session
	w1.mu.Unlock()
	sess.watcher.Close()
	sess.manager.Close()
	// A dead process holds no files; Windows can't remove open ones.
	if w1.cfgWatch != nil {
		w1.cfgWatch.Close()
	}
	a1.closer.Close()
	dead := exec.Command(os.Args[0], "-test.run=^$")
	dead.Run()
	lockFile := filepath.Join(home, "cache", "journal", sess.manager.Project().Hash, "session.lock")
	if err := os.WriteFile(lockFile, []byte(strconv.Itoa(dead.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}

	a2, err := NewApp(Options{ProjectPath: root})
	if err != nil {
		t.Fatal(err)
	}
	a2.start(context.Background(), events.Nop{}, nil)
	defer a2.Shutdown(context.Background())
	got, err := a2.appSvc.PendingRestore(context.Background())
	if err != nil || len(got) != 1 || got[0].Path != file || got[0].Content != "module changed\n" {
		t.Fatalf("PendingRestore = %+v, %v", got, err)
	}
	if err := a2.appSvc.JournalDrop(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if got, _ := a2.appSvc.PendingRestore(context.Background()); len(got) != 0 {
		t.Errorf("after drop = %+v", got)
	}
}

func TestProjectRootForAndPositionalArgs(t *testing.T) {
	root := module(t)
	file := filepath.Join(root, "pkg", "util", "util.go")
	if got := ProjectRootFor(file); got != root {
		t.Errorf("ProjectRootFor = %q, want %q", got, root)
	}
	loose := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(loose, nil, 0o644)
	if got := ProjectRootFor(loose); got != filepath.Dir(loose) {
		t.Errorf("loose file root = %q", got)
	}
	opts, err := ParseArgs([]string{file, root})
	if err != nil || opts.ProjectPath != root || len(opts.Files) != 1 || opts.Files[0] != file {
		t.Errorf("opts = %+v, %v", opts, err)
	}
}

func TestOpenFileQueuedThenOpensProject(t *testing.T) {
	root := module(t)
	t.Setenv("GOI_HOME", t.TempDir())
	a, err := NewApp(Options{})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "pkg", "util", "util.go")
	a.OpenFile(file) // before startup: queued
	bus := &events.Recorder{}
	a.start(context.Background(), bus, nil)
	defer a.Shutdown(context.Background())
	if p := a.workspaceSvc.Project(context.Background()); p == nil || p.Root != root {
		t.Fatalf("project = %+v", p)
	}
	ev := bus.Events(TopicOpenFile)
	if len(ev) != 1 || ev[0].Payload.(OpenFileEvent).Path != file {
		t.Errorf("open file events = %+v", ev)
	}
}

func TestWriteUserSettingsFileOutsideProject(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	paths, err := a.settingsSvc.Paths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	user := paths.User
	if _, err := a.fileSvc.Write(context.Background(), user, "[editor]\nfontSize = 15\n", vfs.FileMeta{}); err != nil {
		t.Fatalf("write user settings: %v", err)
	}
	if _, err := a.fileSvc.Write(context.Background(), filepath.Join(filepath.Dir(user), "other.toml"), "x", vfs.FileMeta{}); err == nil {
		t.Error("other file outside project: want error")
	}
}

// fakeHandle is a windowHandle for tests.
type fakeHandle struct {
	focus func()
	title func(string)
}

func (h *fakeHandle) Focus() {
	if h.focus != nil {
		h.focus()
	}
}

func (h *fakeHandle) SetTitle(s string) {
	if h.title != nil {
		h.title(s)
	}
}

func (h *fakeHandle) Close() {}

// mainWindow returns the first window, the one started() creates.
func (a *App) mainWindow() *window { return a.allWindows()[0] }

// current returns the first window's session.
func (a *App) current() (*session, error) { return a.mainWindow().current() }

// openProject opens path in the first window.
func (a *App) openProject(path string) error { return a.mainWindow().openProject(path) }

// closeProject closes the first window's project.
func (a *App) closeProject() error { return a.mainWindow().closeProject() }
