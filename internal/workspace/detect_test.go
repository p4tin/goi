package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/p4tin/goi/internal/vfs"
)

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetect(t *testing.T) {
	type want struct {
		kind    Kind
		modules []string // module paths
		errs    int
		errLine int
	}
	tests := []struct {
		name   string
		files  map[string]string
		gopath bool
		want   want
	}{
		{
			name: "workspace",
			files: map[string]string{
				"go.work":  "go 1.26\n\nuse (\n\t./a\n\t./b\n)\n",
				"a/go.mod": "module example.com/a\n\ngo 1.26\n",
				"b/go.mod": "module example.com/b\n\ngo 1.25\n",
				"c/go.mod": "module example.com/c\n", // not in go.work
			},
			want: want{kind: KindWorkspace, modules: []string{"example.com/a", "example.com/b"}},
		},
		{
			name: "workspace with missing module",
			files: map[string]string{
				"go.work":  "go 1.26\nuse ./a\nuse ./gone\n",
				"a/go.mod": "module example.com/a\n",
			},
			want: want{kind: KindWorkspace, modules: []string{"example.com/a"}, errs: 1},
		},
		{
			name:  "module",
			files: map[string]string{"go.mod": "module example.com/m\n\ngo 1.26.0\n", "sub/go.mod": "module example.com/sub\n"},
			want:  want{kind: KindModule, modules: []string{"example.com/m"}},
		},
		{
			name: "multi-module",
			files: map[string]string{
				"svc/api/go.mod":        "module example.com/api\n",
				"svc/worker/go.mod":     "module example.com/worker\n",
				"vendor/x/go.mod":       "module skipped/vendor\n",
				"testdata/y/go.mod":     "module skipped/testdata\n",
				"node_modules/z/go.mod": "module skipped/excluded\n",
				"a/b/c/d/e/go.mod":      "module skipped/toodeep\n",
				"a/b/c/d/go.mod":        "module example.com/deep\n",
				".hidden/go.mod":        "module skipped/hidden\n",
			},
			want: want{kind: KindMultiModule, modules: []string{"example.com/deep", "example.com/api", "example.com/worker"}},
		},
		{
			name:  "broken go.mod falls back to folder",
			files: map[string]string{"go.mod": "module example.com/m\n\ngo 1.26\nrequire (\n\tbad line here\n)\n"},
			want:  want{kind: KindFolder, errs: 1, errLine: 5},
		},
		{
			name:  "broken go.work falls back to folder",
			files: map[string]string{"go.work": "go 1.26\nuse (\n", "go.mod": "module example.com/m\n"},
			want:  want{kind: KindFolder, errs: 1},
		},
		{
			name:  "go.mod without module directive",
			files: map[string]string{"go.mod": "go 1.26\n"},
			want:  want{kind: KindFolder, errs: 1, errLine: 1},
		},
		{
			name:   "gopath",
			files:  map[string]string{"main.go": "package main\n"},
			gopath: true,
			want:   want{kind: KindGopath},
		},
		{
			name:  "plain folder",
			files: map[string]string{"README.md": "hi\n"},
			want:  want{kind: KindFolder},
		},
	}
	m, err := vfs.NewMatcher([]string{"node_modules"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			root := base
			var gopath []string
			if tt.gopath {
				root = filepath.Join(base, "src", "example.com", "proj")
				gopath = []string{base}
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, root, tt.files)
			p, err := Detect(context.Background(), root, gopath, m)
			if err != nil {
				t.Fatal(err)
			}
			if p.Kind != tt.want.kind {
				t.Errorf("kind = %s, want %s (errors %+v)", p.Kind, tt.want.kind, p.ParseErrors)
			}
			var got []string
			for _, mod := range p.Modules {
				got = append(got, mod.Path)
			}
			if len(got) != len(tt.want.modules) {
				t.Fatalf("modules = %v, want %v", got, tt.want.modules)
			}
			for i := range got {
				if got[i] != tt.want.modules[i] {
					t.Errorf("modules = %v, want %v", got, tt.want.modules)
				}
			}
			if len(p.ParseErrors) != tt.want.errs {
				t.Errorf("parse errors = %+v, want %d", p.ParseErrors, tt.want.errs)
			}
			if tt.want.errLine > 0 && len(p.ParseErrors) > 0 && p.ParseErrors[0].Line != tt.want.errLine {
				t.Errorf("error line = %d, want %d (%+v)", p.ParseErrors[0].Line, tt.want.errLine, p.ParseErrors[0])
			}
			if p.Hash == "" || len(p.Hash) != 16 || p.ContentRoots[0] != p.Root {
				t.Errorf("hash/contentRoots = %q %v", p.Hash, p.ContentRoots)
			}
		})
	}
}

func TestDetectGoVersionAndDir(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"go.mod": "module example.com/m\n\ngo 1.26.0\n"})
	p, err := Detect(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Modules[0].GoVersion != "1.26.0" || p.Modules[0].Dir != p.Root {
		t.Errorf("module = %+v", p.Modules[0])
	}
}

func TestDetectErrors(t *testing.T) {
	if _, err := Detect(context.Background(), filepath.Join(t.TempDir(), "missing"), nil, nil); err == nil {
		t.Error("missing root: want error")
	}
	f := filepath.Join(t.TempDir(), "file")
	os.WriteFile(f, nil, 0o644)
	if _, err := Detect(context.Background(), f, nil, nil); err == nil {
		t.Error("file root: want error")
	}
}

func TestHashStableAcrossSymlink(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if Hash(real) != Hash(link) {
		t.Error("hash differs for symlinked root")
	}
}
