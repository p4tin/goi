package rpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/gopls"
	"github.com/p4tin/goi/internal/lsp"
	"github.com/p4tin/goi/internal/settings"
)

func TestMain(m *testing.M) {
	// Most tests open projects; don't spawn a real gopls for each one.
	startLangHook = nil
	os.Exit(m.Run())
}

func withLang(t *testing.T) {
	t.Helper()
	startLangHook = startLang
	t.Cleanup(func() { startLangHook = nil })
}

func waitLangStatus(t *testing.T, bus *events.Recorder, state string) LangStatus {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		evs := bus.Events(events.TopicLangStatus)
		if len(evs) > 0 {
			if st := evs[len(evs)-1].Payload.(LangStatus); st.State == state {
				return st
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no %s status; got %+v", state, bus.Events(events.TopicLangStatus))
	return LangStatus{}
}

func TestLangMissingBinary(t *testing.T) {
	withLang(t)
	a, bus := started(t, "")
	if err := a.mainWindow().settings.Set(settings.LayerUser, "gopls.path", filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatal(err)
	}
	if err := a.openProject(module(t)); err != nil {
		t.Fatal(err)
	}
	st := waitLangStatus(t, bus, LangMissing)
	if !strings.Contains(st.Message, "gopls") {
		t.Errorf("message = %q", st.Message)
	}
	if got := a.langSvc.Status(); got.State != LangMissing {
		t.Errorf("Status() = %+v", got)
	}
	if _, err := a.langSvc.Definition(filepath.Join(t.TempDir(), "x.go"), 0, 0); err == nil {
		t.Error("request without gopls should fail")
	}
}

func TestLangDisabled(t *testing.T) {
	withLang(t)
	a, bus := started(t, "")
	if err := a.mainWindow().settings.Set(settings.LayerUser, "gopls.enabled", false); err != nil {
		t.Fatal(err)
	}
	if err := a.openProject(module(t)); err != nil {
		t.Fatal(err)
	}
	waitLangStatus(t, bus, LangDisabled)
}

func TestLangToEditorMapsRealRoot(t *testing.T) {
	ls := &langSession{root: "/link/proj", realRoot: "/real/proj"}
	sep := string(filepath.Separator)
	cases := map[string]string{
		"/real/proj":                     "/link/proj",
		"/real/proj" + sep + "a.go":      "/link/proj" + sep + "a.go",
		"/real/projector" + sep + "a.go": "/real/projector" + sep + "a.go",
		"/elsewhere/b.go":                "/elsewhere/b.go",
	}
	for in, want := range cases {
		if got := ls.toEditor(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
}

func TestApplyFileEditsPreservesCRLF(t *testing.T) {
	a, _ := started(t, "")
	p := filepath.Join(t.TempDir(), "a.go")
	os.WriteFile(p, []byte("package a\r\n\r\nvar x = 1\r\n"), 0o644)
	err := a.langSvc.ApplyFileEdits([]gopls.FileEdit{{Path: p, Edits: []lsp.TextEdit{{
		Range:   lsp.Range{Start: lsp.Position{Line: 2, Character: 4}, End: lsp.Position{Line: 2, Character: 5}},
		NewText: "y",
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "package a\r\n\r\nvar y = 1\r\n" {
		t.Errorf("got %q", b)
	}
}

func waitIndex(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if sess, err := a.current(); err == nil {
			if _, ok := sess.search.paths(); ok {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("index not ready")
}

func TestFindInPathScopeAndBuffers(t *testing.T) {
	a, _ := started(t, "")
	root := module(t)
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n// needle main\n"), 0o644)
	util := filepath.Join(root, "pkg", "util", "util.go")
	os.WriteFile(util, []byte("package helpers\n// needle util\n"), 0o644)
	if err := a.openProject(root); err != nil {
		t.Fatal(err)
	}
	waitIndex(t, a)

	res, err := a.searchSvc.FindInPath(FindQuery{Pattern: "needle"})
	if err != nil || len(res.Matches) != 2 {
		t.Fatalf("all: %+v %v", res, err)
	}
	res, _ = a.searchSvc.FindInPath(FindQuery{Pattern: "needle", Scope: filepath.Join(root, "pkg")})
	if len(res.Matches) != 1 || res.Matches[0].Path != util {
		t.Fatalf("scoped: %+v", res)
	}
	res, _ = a.searchSvc.FindInPath(FindQuery{Pattern: "needle", Buffers: []BufferText{{Path: util, Text: "package helpers\n"}}})
	if len(res.Matches) != 1 || res.Matches[0].Path == util {
		t.Fatalf("buffer override: %+v", res)
	}
	res, _ = a.searchSvc.FindInPath(FindQuery{Pattern: "needle", Mask: "!util.go"})
	if len(res.Matches) != 1 {
		t.Fatalf("mask: %+v", res)
	}
	if _, err := a.searchSvc.FindInPath(FindQuery{Pattern: "(", Regex: true}); err == nil {
		t.Error("bad regex accepted")
	}

	n, err := a.searchSvc.ReplaceInPath(FindQuery{Pattern: "needle", Replace: "pin"}, []string{util, filepath.Join(t.TempDir(), "outside.go")})
	if n != 1 || err == nil || !strings.Contains(err.Error(), "outside the project") {
		t.Fatalf("replace = %d, %v", n, err)
	}
	b, _ := os.ReadFile(util)
	if !strings.Contains(string(b), "// pin util") {
		t.Errorf("util = %q", b)
	}
}
