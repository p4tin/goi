package golang

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleDirFor(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "calc"), 0o755)
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644)
	os.WriteFile(filepath.Join(root, "calc", "calc.go"), []byte("package calc\n"), 0o644)
	if got := moduleDirFor(filepath.Join(root, "calc", "calc.go"), root); got != root {
		t.Fatalf("moduleDirFor = %q", got)
	}
	if got := moduleDirFor(t.TempDir(), root); got != "" {
		t.Fatalf("outside = %q", got)
	}
}
