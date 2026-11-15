package gopls

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/langserver"
	"github.com/p4tin/goi/internal/lsp"
)

func TestLanguageID(t *testing.T) {
	for in, want := range map[string]string{"a.go": "go", "/x/go.mod": "go.mod", "go.work": "go.work", "go.sum": "go.sum", "a.txt": ""} {
		if got := LanguageID(in); got != want {
			t.Errorf("%s: %q", in, got)
		}
	}
}

func writeExec(t *testing.T, path string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestResolveOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec bits")
	}
	root := t.TempDir()
	notOnPath := func(string) (string, error) { return "", errors.New("no") }
	gobin := filepath.Join(root, "gobin")
	gp1, gp2 := filepath.Join(root, "gp1"), filepath.Join(root, "gp2")
	tools := filepath.Join(root, "tools")
	opts := ResolveOptions{LookPath: notOnPath, GOBIN: gobin, GOPATH: gp1 + string(filepath.ListSeparator) + gp2, ToolsDir: tools}

	if _, err := Resolve(opts); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty: %v", err)
	}
	writeExec(t, filepath.Join(tools, "gopls"))
	if p, _ := Resolve(opts); p != filepath.Join(tools, "gopls") {
		t.Errorf("tools: %q", p)
	}
	writeExec(t, filepath.Join(gp2, "bin", "gopls"))
	if p, _ := Resolve(opts); p != filepath.Join(gp2, "bin", "gopls") {
		t.Errorf("gopath2: %q", p)
	}
	writeExec(t, filepath.Join(gobin, "gopls"))
	if p, _ := Resolve(opts); p != filepath.Join(gobin, "gopls") {
		t.Errorf("gobin: %q", p)
	}
	opts.LookPath = func(string) (string, error) { return "/on/path/gopls", nil }
	if p, _ := Resolve(opts); p != "/on/path/gopls" {
		t.Errorf("path: %q", p)
	}
	opts.Setting = filepath.Join(root, "custom")
	if _, err := Resolve(opts); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad setting: %v", err)
	}
	writeExec(t, opts.Setting)
	if p, _ := Resolve(opts); p != opts.Setting {
		t.Errorf("setting: %q", p)
	}
}

// TestRealGopls exercises the real gopls binary when it is installed.
func TestRealGopls(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go")
	}
	out, _ := exec.Command(goBin, "env", "GOBIN", "GOPATH").Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	opts := ResolveOptions{}
	if len(lines) == 2 {
		opts.GOBIN, opts.GOPATH = lines[0], lines[1]
	}
	bin, err := Resolve(opts)
	if err != nil {
		t.Skip("gopls not installed")
	}
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.22\n"), 0o644)
	src := "package main\n\nimport \"fmt\"\n\nfunc helper() int { return 1 }\n\nfunc main() {\n\t_ = helper()\n\tfmt.Println()\n}\n"
	main := filepath.Join(root, "main.go")
	os.WriteFile(main, []byte(src), 0o644)

	diags := make(chan string, 64)
	s := langserver.New(langserver.Config{Name: "gopls", LanguageID: LanguageID, Bin: bin, Root: root, Env: os.Environ(),
		OnDiagnostics: func(path string, _ []lsp.Diagnostic) {
			select {
			case diags <- path:
			default:
			}
		}})
	s.Start()
	defer s.Close()
	s.Open(main, 1, src)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	locs, err := s.Definition(ctx, main, lsp.Position{Line: 7, Character: 6})
	if err != nil || len(locs) != 1 || locs[0].Path != main || locs[0].Range.Start.Line != 4 {
		t.Fatalf("definition = %+v, %v", locs, err)
	}

	edited := strings.Replace(src, "\tfmt.Println()\n", "\tfmt.Pr\n", 1)
	s.Change(main, 2, edited)
	list, err := s.Completion(ctx, main, lsp.Position{Line: 8, Character: 7})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range list.Items {
		if it.Label == "Println" || it.Label == "Printf" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no Print* completion in %d items", len(list.Items))
	}
	deadline := time.After(30 * time.Second)
	for {
		select {
		case p := <-diags:
			if p == main {
				return
			}
		case <-deadline:
			t.Fatal("no diagnostics for broken code")
		}
	}
}

// TestCompletionLatency measures completion on this repository with the real
// gopls (R12.2). Opt-in: GOI_GOPLS_BENCH=1 go test -run CompletionLatency -v.
func TestCompletionLatency(t *testing.T) {
	if os.Getenv("GOI_GOPLS_BENCH") != "1" {
		t.Skip("set GOI_GOPLS_BENCH=1")
	}
	out, _ := exec.Command("go", "env", "GOBIN", "GOPATH").Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	opts := ResolveOptions{}
	if len(lines) == 2 {
		opts.GOBIN, opts.GOPATH = lines[0], lines[1]
	}
	bin, err := Resolve(opts)
	if err != nil {
		t.Skip("gopls not installed")
	}
	root, _ := filepath.Abs("../../..")
	file := filepath.Join(root, "internal", "rpc", "lang.go")
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	marker := "ls.app.bus.Emit(events.TopicLangDiagnostics"
	off := strings.Index(text, marker) + len("ls.app.")
	line := strings.Count(text[:off], "\n")
	char := off - (strings.LastIndex(text[:off], "\n") + 1)

	s := langserver.New(langserver.Config{Name: "gopls", LanguageID: LanguageID, Bin: bin, Root: root, Env: os.Environ()})
	s.Start()
	defer s.Close()
	s.Open(file, 1, text)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.Completion(ctx, file, lsp.Position{Line: line, Character: char}); err != nil { // warm-up
		t.Fatal(err)
	}
	var times []time.Duration
	for range 20 {
		start := time.Now()
		list, err := s.Completion(ctx, file, lsp.Position{Line: line, Character: char})
		if err != nil || len(list.Items) == 0 {
			t.Fatalf("completion: %d items, %v", len(list.Items), err)
		}
		times = append(times, time.Since(start))
	}
	slices.Sort(times)
	t.Logf("completion on %s: p50=%s p95=%s", filepath.Base(root), times[10], times[18])
	if times[18] > 150*time.Millisecond {
		t.Errorf("p95 %s exceeds 150ms", times[18])
	}
}
