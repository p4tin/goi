package pyright

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/langserver"
	"github.com/p4tin/goi/internal/lsp"
	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/settings"
)

func TestSectionAndLanguageID(t *testing.T) {
	cfg := (Server{}).Settings(pack.ProjectEnv{Root: t.TempDir()})
	if a, ok := (Server{}).Section("basedpyright.analysis", cfg).(map[string]any); !ok || a["autoSearchPaths"] != true {
		t.Fatalf("analysis section = %v", (Server{}).Section("basedpyright.analysis", cfg))
	}
	if m, ok := (Server{}).Section("nope.x", cfg).(map[string]any); !ok || len(m) != 0 {
		t.Fatal("unknown section must be an empty object")
	}
	if LanguageID("a/b.py") != "python" || LanguageID("x.pyi") != "python" || LanguageID("x.go") != "" {
		t.Fatal("LanguageID")
	}
	if !(Server{}).Enabled(settings.Settings{}) {
		t.Fatal("enabled by default")
	}
}

// TestRealBasedpyright checks completion and definition with a real
// basedpyright-langserver (GOI_PYRIGHT=<path>, or one on PATH).
func TestRealBasedpyright(t *testing.T) {
	bin := os.Getenv("GOI_PYRIGHT")
	if bin == "" {
		var err error
		if bin, err = exec.LookPath(Binary); err != nil {
			t.Skip("basedpyright-langserver not installed (set GOI_PYRIGHT)")
		}
	}
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	src := "import os\n\ndef helper(n: int) -> int:\n    return n * 2\n\nprint(helper(2))\nos.pa\n"
	file := filepath.Join(root, "main.py")
	os.WriteFile(file, []byte(src), 0o644)
	sp := Server{}
	s := langserver.New(langserver.Config{Name: sp.Name(), Bin: bin, Args: []string{"--stdio"}, Root: root, Env: os.Environ(),
		Settings: sp.Settings(pack.ProjectEnv{Root: root}), Section: sp.Section, LanguageID: LanguageID})
	s.Start()
	defer s.Close()
	s.Open(file, 1, src)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	locs, err := s.Definition(ctx, file, lsp.Position{Line: 5, Character: 8})
	if err != nil || len(locs) == 0 || locs[0].Range.Start.Line != 2 {
		t.Fatalf("definition = %+v, %v", locs, err)
	}
	list, err := s.Completion(ctx, file, lsp.Position{Line: 6, Character: 5})
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, it := range list.Items {
		labels = append(labels, it.Label)
	}
	if !strings.Contains(strings.Join(labels, ","), "path") {
		t.Fatalf("no os.path completion in %v", labels)
	}
}
