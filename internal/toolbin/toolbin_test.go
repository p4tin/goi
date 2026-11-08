package toolbin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeExe(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, ExeName(name))
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveOrder(t *testing.T) {
	base := t.TempDir()
	gobin, gp, tools := filepath.Join(base, "gobin"), filepath.Join(base, "gp"), filepath.Join(base, "tools")
	notOnPath := func(string) (string, error) { return "", errors.New("no") }
	o := Options{Name: "dlv", LookPath: notOnPath, GOBIN: gobin, GOPATH: gp, ToolsDir: tools}

	_, err := Resolve(o)
	if !errors.Is(err, ErrNotFound) || err.Error() != "dlv not found" {
		t.Fatalf("want dlv not found, got %v", err)
	}
	want := writeExe(t, tools, "dlv")
	if p, _ := Resolve(o); p != want {
		t.Fatalf("tools dir: got %q want %q", p, want)
	}
	want = writeExe(t, filepath.Join(gp, "bin"), "dlv")
	if p, _ := Resolve(o); p != want {
		t.Fatalf("GOPATH/bin: got %q want %q", p, want)
	}
	want = writeExe(t, gobin, "dlv")
	if p, _ := Resolve(o); p != want {
		t.Fatalf("GOBIN: got %q want %q", p, want)
	}
	o.LookPath = func(f string) (string, error) { return "/on/path/" + f, nil }
	if p, _ := Resolve(o); p != "/on/path/"+ExeName("dlv") {
		t.Fatalf("PATH: got %q", p)
	}
	o.Setting = filepath.Join(base, "missing")
	if _, err := Resolve(o); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad setting: want ErrNotFound, got %v", err)
	}
	o.Setting = want
	if p, _ := Resolve(o); p != want {
		t.Fatalf("setting: got %q", p)
	}
}

func TestIsExec(t *testing.T) {
	dir := t.TempDir()
	if IsExec(dir) {
		t.Fatal("dir is not executable")
	}
	p := filepath.Join(dir, "plain")
	os.WriteFile(p, nil, 0o644)
	if runtime.GOOS != "windows" && IsExec(p) {
		t.Fatal("non-executable file reported executable")
	}
}

func TestWithoutKeysAndTail(t *testing.T) {
	got := WithoutKeys([]string{"A=1", "GOBIN=x", "GOFLAGS=-mod=mod", "B=2"}, "GOBIN", "GOFLAGS")
	if len(got) != 2 || got[0] != "A=1" || got[1] != "B=2" {
		t.Fatalf("WithoutKeys = %v", got)
	}
	if Tail("abcdef", 3) != "…def" || Tail("ab", 3) != "ab" {
		t.Fatal("Tail")
	}
}

func TestInstallUsesAbsoluteGOBIN(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the go binary")
	}
	t.Chdir(t.TempDir())
	// A fake go that fails unless GOBIN is absolute, then "installs" the tool.
	fakeGo := filepath.Join(t.TempDir(), "go")
	os.WriteFile(fakeGo, []byte("#!/bin/sh\ncase \"$GOBIN\" in /*) ;; *) echo 'GOBIN must be an absolute path'; exit 1;; esac\nprintf '#!/bin/sh\\n' > \"$GOBIN/dlv\"; chmod +x \"$GOBIN/dlv\"\n"), 0o755)
	p, err := Install(context.Background(), fakeGo, os.Environ(), filepath.Join("rel", "tools"), "example.com/dlv@latest", "dlv")
	if err != nil {
		t.Fatalf("Install with a relative tools dir: %v", err)
	}
	if !filepath.IsAbs(p) {
		t.Fatalf("installed path %q is not absolute", p)
	}
}
