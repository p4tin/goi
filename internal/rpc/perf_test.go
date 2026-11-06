package rpc

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPerfOpenLargeProject measures NFR-2: opening a 10k-file project until
// the tree's first listing and the search index are ready.
func TestPerfOpenLargeProject(t *testing.T) {
	if testing.Short() {
		t.Skip("perf")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/big\n"), 0o644)
	for i := 0; i < 10_000; i++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%03d", i/100))
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d.go", i)), []byte("package p\n"), 0o644)
	}
	start := time.Now()
	a, _ := started(t, root)
	opened := time.Since(start)
	if _, err := a.fileSvc.List(root); err != nil {
		t.Fatal(err)
	}
	listed := time.Since(start)
	eventuallyRPC(t, func() bool {
		r, _ := a.searchSvc.Files("f09999", 1, nil)
		return r.Index.Ready && len(r.Matches) == 1
	})
	indexed := time.Since(start)
	t.Logf("10k files: open %v, tree listing %v, index ready %v", opened, listed, indexed)
	if listed > time.Second {
		t.Errorf("tree interactive after %v, want < 1s", listed)
	}
}
