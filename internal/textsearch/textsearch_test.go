package textsearch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCompile(t *testing.T) {
	cases := []struct {
		q          Query
		in         string
		wantMatchN int
	}{
		{Query{Pattern: "a.b"}, "a.b axb", 1},
		{Query{Pattern: "a.b", Regex: true}, "a.b axb", 2},
		{Query{Pattern: "Foo"}, "foo FOO Foo", 3},
		{Query{Pattern: "Foo", CaseSensitive: true}, "foo FOO Foo", 1},
		{Query{Pattern: "id", WholeWord: true}, "id idx _id id2 (id)", 2},
		{Query{Pattern: "a|b", Regex: true, WholeWord: true}, "a b ab", 2},
	}
	for _, c := range cases {
		re, err := Compile(c.q)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(re.FindAllStringIndex(c.in, -1)); n != c.wantMatchN {
			t.Errorf("%+v on %q: %d matches, want %d", c.q, c.in, n, c.wantMatchN)
		}
	}
	if _, err := Compile(Query{}); !errors.Is(err, ErrEmptyPattern) {
		t.Errorf("empty: %v", err)
	}
	if _, err := Compile(Query{Pattern: "(", Regex: true}); err == nil {
		t.Error("bad regex accepted")
	}
}

func TestMask(t *testing.T) {
	m := ParseMask("*.go, !*_test.go , ")
	for p, want := range map[string]bool{"/a/x.go": true, "/a/x_test.go": false, "/a/x.md": false} {
		if got := m.Matches(p); got != want {
			t.Errorf("%s: %v", p, got)
		}
	}
	if !ParseMask("").Matches("anything") {
		t.Error("empty mask should match")
	}
	if ParseMask("!*.md").Matches("r.md") || !ParseMask("!*.md").Matches("r.go") {
		t.Error("exclude-only mask")
	}
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.go", "package a\n\nfunc Foo() {}\r\n// foo here: Foo\n")
	b := write(t, dir, "b_test.go", "package a\nvar x = Foo\n")
	bin := write(t, dir, "bin.go", "Foo\x00\x01")
	emoji := write(t, dir, "e.go", "s := \"😀\" + Foo\n")

	got, truncated, err := Search(context.Background(), []string{a, b, bin, emoji}, Query{Pattern: "Foo", CaseSensitive: true, Mask: "*.go, !*_test.go"}, 0)
	if err != nil || truncated {
		t.Fatal(err, truncated)
	}
	if len(got) != 3 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if got[0].Path != a || got[0].Line != 3 || got[0].Col != 5 || got[0].End != 8 || got[0].Preview != "func Foo() {}" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Line != 4 || got[1].Col != 13 {
		t.Errorf("second = %+v", got[1])
	}
	if got[2].Path != emoji || got[2].Col != 12 { // 😀 is two UTF-16 units
		t.Errorf("emoji = %+v", got[2])
	}

	got, truncated, _ = Search(context.Background(), []string{a, b}, Query{Pattern: "foo"}, 2)
	if !truncated || len(got) != 2 {
		t.Errorf("limit: %d %v", len(got), truncated)
	}
}

func TestSearchLongLinePreview(t *testing.T) {
	line := strings.Repeat("x", 500) + "NEEDLE" + strings.Repeat("y", 500)
	got, _, _ := SearchText("f", line, Query{Pattern: "NEEDLE"}, 0)
	if len(got) != 1 {
		t.Fatal(got)
	}
	m := got[0]
	if len(m.Preview) > 400 || m.PreviewStart == 0 || m.Preview[m.Col-m.PreviewStart:m.End-m.PreviewStart] != "NEEDLE" {
		t.Errorf("preview window wrong: start=%d col=%d len=%d", m.PreviewStart, m.Col, len(m.Preview))
	}
}

func TestSearchCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Search(ctx, []string{"x"}, Query{Pattern: "x"}, 0); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestReplaceText(t *testing.T) {
	cases := []struct {
		q         Query
		in, want  string
		wantCount int
	}{
		{Query{Pattern: "foo", Replace: "bar"}, "foo Foo\r\nfoo\n", "bar bar\r\nbar\n", 3},
		{Query{Pattern: "$x", Replace: "$y"}, "a $x b", "a $y b", 1},
		{Query{Pattern: `(\w+)=(\w+)`, Regex: true, Replace: "$2=$1"}, "a=b\nc=d", "b=a\nd=c", 2},
		{Query{Pattern: `^`, Regex: true, Replace: "#"}, "a\nb", "a\nb", 0},
		{Query{Pattern: "id", WholeWord: true, CaseSensitive: true, Replace: "ID"}, "id idx id", "ID idx ID", 2},
		{Query{Pattern: `o$`, Regex: true, Replace: "0"}, "foo\r\nboo\n", "fo0\r\nbo0\n", 2},
	}
	for _, c := range cases {
		re, err := Compile(c.q)
		if err != nil {
			t.Fatal(err)
		}
		got, n := ReplaceText(c.in, re, Template(c.q))
		if got != c.want || n != c.wantCount {
			t.Errorf("%+v: got %q (%d), want %q (%d)", c.q, got, n, c.want, c.wantCount)
		}
	}
}
