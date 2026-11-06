package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/p4tin/goi/internal/vfs"
)

func tree(t testing.TB, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func build(t testing.TB, root string, excludes ...string) *Index {
	t.Helper()
	m, err := vfs.NewMatcher(excludes)
	if err != nil {
		t.Fatal(err)
	}
	ix, err := Build(context.Background(), []string{root}, m)
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func rels(ms []Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Rel
	}
	return out
}

func TestBuildRespectsExcludes(t *testing.T) {
	root := tree(t, "a.go", "pkg/b.go", ".git/HEAD", "node_modules/x/y.js", "pkg/.git/config")
	ix := build(t, root, ".git", "node_modules")
	if ix.Len() != 2 {
		t.Fatalf("Len = %d, want 2", ix.Len())
	}
}

func TestSearchRanking(t *testing.T) {
	tests := []struct {
		query string
		files []string
		top   string
	}{
		{"wsmgr", []string{"internal/workspace_manager.go", "internal/ws/messages.go", "docs/settings.md"}, "internal/workspace_manager.go"},
		// A literal substring in the basename beats a scattered match.
		{"wsmgr", []string{"internal/workspace_manager.go", "docs/wsmgr_notes.txt"}, "docs/wsmgr_notes.txt"},
		{"main", []string{"internal/domain/xmain.go", "cmd/goi/main.go", "maintenance/readme.md"}, "cmd/goi/main.go"},
		{"wm", []string{"warm.go", "internal/WorkspaceManager.go"}, "internal/WorkspaceManager.go"},
		{"app go", []string{"internal/rpc/app.go", "frontend/src/app.ts", "apple/go.mod"}, "internal/rpc/app.go"},
		{"store", []string{"internal/settings/store.go", "frontend/src/lib/store.ts", "s/t/o/r/e.go"}, "internal/settings/store.go"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			ix := build(t, tree(t, tt.files...))
			got := ix.Search(tt.query, 10, nil)
			if len(got) == 0 || got[0].Rel != tt.top {
				t.Errorf("top = %v, want %q first", rels(got), tt.top)
			}
		})
	}
}

func TestSearchNoMatchAndPositions(t *testing.T) {
	ix := build(t, tree(t, "internal/workspace_manager.go"))
	if got := ix.Search("zzz", 10, nil); len(got) != 0 {
		t.Errorf("got %v", rels(got))
	}
	got := ix.Search("wsmgr", 10, nil)
	if len(got) != 1 {
		t.Fatalf("got %v", rels(got))
	}
	rel := got[0].Rel
	var picked []byte
	for _, p := range got[0].Positions {
		picked = append(picked, rel[p])
	}
	if string(picked) != "wsmgr" {
		t.Errorf("positions pick %q from %q", picked, rel)
	}
}

func TestRecentBoostAndEmptyQuery(t *testing.T) {
	root := tree(t, "a/config.go", "b/config.go")
	ix := build(t, root)
	recent := []string{filepath.Join(root, "b", "config.go")}
	got := ix.Search("config", 10, recent)
	if len(got) != 2 || got[0].Rel != "b/config.go" {
		t.Errorf("got %v, want b/config.go first", rels(got))
	}
	empty := ix.Search("", 10, append(recent, filepath.Join(root, "missing.go")))
	if len(empty) != 1 || empty[0].Rel != "b/config.go" {
		t.Errorf("empty query = %v", rels(empty))
	}
}

func TestAddRemove(t *testing.T) {
	root := tree(t, "a.go", "dir/b.go", "dir/sub/c.go")
	ix := build(t, root, "vendor")
	ix.Remove(filepath.Join(root, "dir"))
	if ix.Len() != 1 {
		t.Fatalf("after dir remove Len = %d, want 1", ix.Len())
	}
	newFile := filepath.Join(root, "new.go")
	os.WriteFile(newFile, nil, 0o644)
	ix.Add(newFile)
	ix.Add(newFile) // idempotent
	vendored := filepath.Join(root, "vendor", "x", "v.go")
	os.MkdirAll(filepath.Dir(vendored), 0o755)
	os.WriteFile(vendored, nil, 0o644)
	ix.Add(vendored)                        // excluded ancestor
	ix.Add(filepath.Join(root, "nope.go"))  // missing
	ix.Add(filepath.Join(t.TempDir(), "x")) // outside roots
	if ix.Len() != 2 {
		t.Fatalf("Len = %d, want 2", ix.Len())
	}
	if got := ix.Search("new", 5, nil); len(got) != 1 || got[0].Rel != "new.go" {
		t.Errorf("got %v", rels(got))
	}
}

func syntheticIndex(n int) *Index {
	ix := &Index{roots: []string{"/r"}, m: vfs.MatchNone, pos: map[string]int{}}
	dirs := []string{"internal", "cmd", "frontend/src", "pkg", "vendor/github.com/acme", "docs", "test/fixtures"}
	words := []string{"workspace", "manager", "settings", "store", "index", "render", "parser", "token", "handler", "client"}
	for i := 0; i < n; i++ {
		rel := fmt.Sprintf("%s/%s_%d/%s_%s_%d.go", dirs[i%len(dirs)], words[(i/7)%len(words)], i%97, words[i%len(words)], words[(i/3)%len(words)], i)
		ix.add(0, "/r/"+rel, rel)
	}
	return ix
}

func TestSearch50kUnder100ms(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark-style test")
	}
	ix := syntheticIndex(50_000)
	res := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ix.Search("wsmgr", 50, nil)
		}
	})
	if per := res.NsPerOp(); per > 100e6 {
		t.Errorf("Search over 50k paths = %d ms/op, want < 100 ms", per/1e6)
	}
}

func BenchmarkSearch50k(b *testing.B) {
	ix := syntheticIndex(50_000)
	for _, q := range []string{"wsmgr", "store", "frontend handler"} {
		b.Run(q, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ix.Search(q, 50, nil)
			}
		})
	}
}
