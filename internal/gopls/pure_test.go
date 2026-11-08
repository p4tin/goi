package gopls

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/p4tin/goi/internal/lsp"
)

func TestURIRoundTrip(t *testing.T) {
	cases := []struct {
		path, uri string
		windows   bool
	}{
		{"/a/b c/x.go", "file:///a/b%20c/x.go", false},
		{`C:\Users\me\x.go`, "file:///C:/Users/me/x.go", true},
		{`d:\w\y.go`, "file:///D:/w/y.go", true},
	}
	for _, c := range cases {
		if got := pathToURI(c.path, c.windows); got != c.uri {
			t.Errorf("pathToURI(%q) = %q, want %q", c.path, got, c.uri)
		}
		want := c.path
		if c.windows && want[0] >= 'a' && want[0] <= 'z' {
			want = string(want[0]-32) + want[1:] // drive letters come back upper-case
		}
		if got := uriToPath(c.uri, c.windows); got != want {
			t.Errorf("uriToPath(%q) = %q, want %q", c.uri, got, want)
		}
	}
	if got := uriToPath("file:///c%3A/x.go", true); got != `C:\x.go` {
		t.Errorf("encoded drive: %q", got)
	}
	if got := uriToPath("untitled:1", false); got != "untitled:1" {
		t.Errorf("non-file: %q", got)
	}
}

func edit(l1, c1, l2, c2 int, text string) lsp.TextEdit {
	return lsp.TextEdit{Range: lsp.Range{Start: lsp.Position{Line: l1, Character: c1}, End: lsp.Position{Line: l2, Character: c2}}, NewText: text}
}

func TestApplyEdits(t *testing.T) {
	cases := []struct {
		name, in string
		edits    []lsp.TextEdit
		want     string
	}{
		{"insert", "ab\ncd\n", []lsp.TextEdit{edit(1, 1, 1, 1, "X")}, "ab\ncXd\n"},
		{"replace across lines", "ab\ncd\n", []lsp.TextEdit{edit(0, 1, 1, 1, "Z")}, "aZd\n"},
		{"multiple unordered", "abc", []lsp.TextEdit{edit(0, 2, 0, 3, "C"), edit(0, 0, 0, 1, "A")}, "AbC"},
		{"same position keeps order", "x", []lsp.TextEdit{edit(0, 0, 0, 0, "1"), edit(0, 0, 0, 0, "2")}, "12x"},
		{"utf16 surrogate", "a😀b", []lsp.TextEdit{edit(0, 3, 0, 4, "B")}, "a😀B"},
		{"bmp multibyte", "héllo", []lsp.TextEdit{edit(0, 2, 0, 3, "L")}, "héLlo"},
		{"clamp past line end", "ab\ncd", []lsp.TextEdit{edit(0, 99, 0, 99, "!")}, "ab!\ncd"},
		{"clamp past eof", "ab", []lsp.TextEdit{edit(5, 0, 5, 0, "\nz")}, "ab\nz"},
		{"crlf line end", "ab\r\ncd", []lsp.TextEdit{edit(0, 9, 0, 9, "!")}, "ab!\r\ncd"},
		{"import insert", "package p\n\nfunc f() {}\n", []lsp.TextEdit{edit(1, 0, 1, 0, "import \"fmt\"\n")}, "package p\nimport \"fmt\"\n\nfunc f() {}\n"},
	}
	for _, c := range cases {
		got, err := ApplyEdits(c.in, c.edits)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if _, err := ApplyEdits("abcdef", []lsp.TextEdit{edit(0, 0, 0, 3, "x"), edit(0, 2, 0, 4, "y")}); !errors.Is(err, ErrOverlappingEdits) {
		t.Errorf("overlap err = %v", err)
	}
}

func TestFileEdits(t *testing.T) {
	v := 1
	we := lsp.WorkspaceEdit{
		Changes: map[string][]lsp.TextEdit{PathToURI(filepath.FromSlash("/p/b.go")): {edit(0, 0, 0, 0, "b")}},
		DocumentChanges: []lsp.TextDocumentEdit{
			{TextDocument: lsp.VersionedTextDocumentIdentifier{URI: PathToURI(filepath.FromSlash("/p/a.go")), Version: &v}, Edits: []lsp.TextEdit{edit(0, 0, 0, 0, "a")}},
			{TextDocument: lsp.VersionedTextDocumentIdentifier{URI: PathToURI(filepath.FromSlash("/p/b.go"))}, Edits: []lsp.TextEdit{edit(1, 0, 1, 0, "b2")}},
		},
	}
	got := FileEdits(we)
	if len(got) != 2 || got[0].Path != filepath.FromSlash("/p/a.go") || got[1].Path != filepath.FromSlash("/p/b.go") || len(got[1].Edits) != 2 {
		t.Fatalf("got %+v", got)
	}
	if out := FileEdits(lsp.WorkspaceEdit{}); out == nil || len(out) != 0 {
		t.Errorf("empty = %#v", out)
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
