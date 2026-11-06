package rpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewProject(t *testing.T) {
	a, _ := started(t, "")
	parent := t.TempDir()

	dir, err := a.appSvc.NewProject(parent, "hello", "example.com/hello")
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(parent, "hello") {
		t.Errorf("dir = %q", dir)
	}
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(mod), "module example.com/hello\n") || !strings.Contains(string(mod), "\ngo 1.") {
		t.Errorf("go.mod = %q", mod)
	}
	main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if string(main) != "package main\n\nfunc main() {\n}\n" {
		t.Errorf("main.go = %q", main)
	}

	if _, err := a.appSvc.NewProject(parent, "hello", "example.com/hello"); err == nil {
		t.Error("existing dir: want error")
	}
	for _, bad := range []string{"", "a/b", `a\b`, "..", "."} {
		if _, err := a.appSvc.NewProject(parent, bad, "example.com/x"); err == nil {
			t.Errorf("name %q: want error", bad)
		}
	}
	if _, err := a.appSvc.NewProject(parent, "ok", "bad path"); err == nil {
		t.Error("module path with space: want error")
	}
	if _, err := a.appSvc.NewProject(filepath.Join(parent, "missing"), "x", "x"); err == nil {
		t.Error("missing parent: want error")
	}
}

func TestGoMajorMinor(t *testing.T) {
	for in, want := range map[string]string{"go1.26.0": "1.26", "go1.27rc1": "1.27", "go1.25": "1.25", "devel abc": "1.26"} {
		if got := goMajorMinor(in); got != want {
			t.Errorf("goMajorMinor(%q) = %q, want %q", in, got, want)
		}
	}
}
