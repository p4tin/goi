package rpc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileServiceReadImage(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	fs := a.fileSvc
	ctx := context.Background()

	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	os.WriteFile(filepath.Join(root, "docs", "logo.png"), []byte("png"), 0o644)
	got, err := fs.ReadImage(ctx, filepath.Join(root, "docs", "logo.png"))
	if err != nil || !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("inside: %q, %v", got, err)
	}

	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.png")
	os.WriteFile(outside, []byte("png"), 0o644)
	if _, err := fs.ReadImage(ctx, outside); err == nil {
		t.Error("outside: want error")
	}
	if _, err := fs.ReadImage(ctx, filepath.Join(root, "..", filepath.Base(outsideDir), "secret.png")); err == nil {
		t.Error(".. escape: want error")
	}
	link := filepath.Join(root, "docs", "link.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := fs.ReadImage(ctx, link); err == nil {
		t.Error("symlink escaping the project: want error")
	}
}
