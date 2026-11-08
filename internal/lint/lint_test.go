package lint

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/p4tin/goi/internal/toolbin"
)

const src = "package lp\n\n// Add adds two numbers\nfunc Add(a, b int) int { return a + b }\n\nfunc é() { unusedVar := 1 }\n"

func files(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := m[p]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}
}

func TestParseV2WithSuggestedFix(t *testing.T) {
	out := `level=warning msg="noise"
{"Issues":[{"FromLinter":"godot","Text":"Comment should end in a period","Severity":"","Pos":{"Filename":"lp.go","Offset":12,"Line":3,"Column":1},"SuggestedFixes":[{"Message":"","TextEdits":[{"Pos":12,"End":35,"NewText":"Ly8gQWRkIGFkZHMgdHdvIG51bWJlcnMu"}]}]},
{"FromLinter":"ineffassign","Text":"ineffectual assignment to unusedVar","Severity":"warning","Pos":{"Filename":"lp.go","Line":6,"Column":13}}],"Report":{}}`
	issues, err := parse([]byte(out), "/p", files(map[string]string{"/p/lp.go": src}))
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %+v", issues)
	}
	a := issues[0]
	if a.Path != "/p/lp.go" || a.Start != (Pos{2, 0}) || a.Linter != "godot" || len(a.Fix) != 1 {
		t.Fatalf("godot = %+v", a)
	}
	if f := a.Fix[0]; f.Start != (Pos{2, 0}) || f.End != (Pos{2, 23}) || f.NewText != "// Add adds two numbers." {
		t.Fatalf("fix = %+v", f)
	}
	// Column 13 (bytes) after "é" (2 bytes) is UTF-16 char 11; the word is unusedVar.
	b := issues[1]
	if b.Start != (Pos{5, 11}) || b.End != (Pos{5, 20}) || b.Severity != "warning" || len(b.Fix) != 0 {
		t.Fatalf("ineffassign = %+v", b)
	}
}

func TestParseV1Replacement(t *testing.T) {
	out := `{"Issues":[{"FromLinter":"misspell","Text":"x","Pos":{"Filename":"/abs/lp.go","Line":3,"Column":4},"Replacement":{"Inline":{"StartCol":3,"Length":3,"NewString":"Sum"}}},
{"FromLinter":"gofmt","Text":"y","Pos":{"Filename":"lp.go","Line":4,"Column":1},"LineRange":{"From":4,"To":4},"Replacement":{"NewLines":["func Add(a, b int) int {", "\treturn a + b", "}"]}},
{"FromLinter":"whitespace","Text":"z","Pos":{"Filename":"lp.go","Line":2,"Column":1},"Replacement":{"NeedOnlyDelete":true}}]}`
	issues, err := parse([]byte(out), "/p", files(map[string]string{"/abs/lp.go": src, "/p/lp.go": src}))
	if err != nil {
		t.Fatal(err)
	}
	if f := issues[0].Fix[0]; f.Start != (Pos{2, 3}) || f.End != (Pos{2, 6}) || f.NewText != "Sum" {
		t.Fatalf("inline = %+v", f)
	}
	if f := issues[1].Fix[0]; f.Start != (Pos{3, 0}) || f.End != (Pos{4, 0}) || f.NewText != "func Add(a, b int) int {\n\treturn a + b\n}\n" {
		t.Fatalf("lines = %+v", f)
	}
	if f := issues[2].Fix[0]; f.NewText != "" || f.End != (Pos{2, 0}) {
		t.Fatalf("delete = %+v", f)
	}
}

func TestParseErrorsAndVersion(t *testing.T) {
	if _, err := Parse([]byte("Error: can't load config"), "/p"); err == nil {
		t.Fatal("want error without JSON")
	}
	for in, want := range map[string]int{"golangci-lint has version 2.15.0 built with go1.26": 2, "golangci-lint has version v1.64.8 built from abc": 1} {
		if m, _, err := parseVersion(in); err != nil || m != want {
			t.Errorf("parseVersion(%q) = %d, %v", in, m, err)
		}
	}
	if _, _, err := parseVersion("nonsense"); err == nil {
		t.Fatal("want version error")
	}
	if got := Args(2, []string{"--fast"}, []string{"./..."}); got[1] != "--output.json.path" || got[len(got)-1] != "./..." || got[len(got)-2] != "--fast" {
		t.Fatalf("v2 args = %v", got)
	}
	if got := Args(1, nil, []string{"."}); got[1] != "--out-format" {
		t.Fatalf("v1 args = %v", got)
	}
}

// TestRealRun runs golangci-lint when GOI_GOLANGCI points at it (or it is
// installed).
func TestRealRun(t *testing.T) {
	bin := os.Getenv("GOI_GOLANGCI")
	if bin == "" {
		var err error
		if bin, err = Resolve(toolbin.Options{GOPATH: os.Getenv("GOPATH")}); err != nil {
			t.Skip("golangci-lint not installed (set GOI_GOLANGCI)")
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lp\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "lp.go"), []byte("package lp\n\nimport \"os\"\n\nfunc F() {\n\tos.Remove(\"x\")\n}\n"), 0o644)
	ctx := context.Background()
	major, _, err := Version(ctx, bin)
	if err != nil {
		t.Fatal(err)
	}
	issues, err := Run(ctx, Spec{Bin: bin, Major: major, Dir: dir, Env: append(os.Environ(), "GOFLAGS=-buildvcs=false"), Targets: []string{"./..."}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, is := range issues {
		if is.Linter == "errcheck" && is.Path == filepath.Join(dir, "lp.go") && is.Start.Line == 5 {
			found = true
		}
	}
	if !found {
		t.Fatalf("errcheck issue missing: %+v", issues)
	}
	// A bad config is an error with golangci-lint's message.
	os.WriteFile(filepath.Join(dir, ".golangci.yml"), []byte("version: \"2\"\nlinters:\n  enable: [definitely-not-a-linter]\n"), 0o644)
	if _, err := Run(ctx, Spec{Bin: bin, Major: major, Dir: dir, Env: os.Environ(), Targets: []string{"./..."}}); err == nil {
		t.Fatal("bad config should fail")
	}
}
