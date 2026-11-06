package format

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.go")
	tests := []struct {
		name    string
		mode    Mode
		src     string
		want    string
		changed bool
	}{
		{
			name:    "gofmt fixes spacing",
			mode:    ModeGofmt,
			src:     "package x\nfunc  f( ) {  }\n",
			want:    "package x\n\nfunc f() {}\n",
			changed: true,
		},
		{
			name: "gofmt leaves formatted source",
			mode: ModeGofmt,
			src:  "package x\n\nfunc f() {}\n",
			want: "package x\n\nfunc f() {}\n",
		},
		{
			name:    "goimports removes unused import",
			mode:    ModeGoimports,
			src:     "package x\n\nimport \"os\"\n\nfunc f() {}\n",
			want:    "package x\n\nfunc f() {}\n",
			changed: true,
		},
		{
			name:    "goimports adds stdlib import",
			mode:    ModeGoimports,
			src:     "package x\n\nfunc f() { fmt.Println() }\n",
			want:    "package x\n\nimport \"fmt\"\n\nfunc f() { fmt.Println() }\n",
			changed: true,
		},
		{
			name:    "goimports sorts and groups",
			mode:    ModeGoimports,
			src:     "package x\n\nimport (\n\t\"strings\"\n\t\"fmt\"\n)\n\nvar _ = fmt.Sprint\nvar _ = strings.ToLower\n",
			want:    "package x\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\n\nvar _ = fmt.Sprint\nvar _ = strings.ToLower\n",
			changed: true,
		},
		{
			name: "none is identity",
			mode: ModeNone,
			src:  "package x\nfunc  f( ) {  }\n",
			want: "package x\nfunc  f( ) {  }\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format(path, tt.src, tt.mode, "")
			if err != nil {
				t.Fatal(err)
			}
			if got.Content != tt.want {
				t.Errorf("content:\n%q\nwant:\n%q", got.Content, tt.want)
			}
			if got.Changed != tt.changed {
				t.Errorf("changed = %v, want %v", got.Changed, tt.changed)
			}
		})
	}
}

func TestFormatLocalPrefix(t *testing.T) {
	src := "package x\n\nimport (\n\t\"example.com/me/a\"\n\t\"github.com/other/b\"\n)\n\nvar _ = a.X\nvar _ = b.Y\n"
	got, err := Format(filepath.Join(t.TempDir(), "x.go"), src, ModeGoimports, "example.com/me")
	if err != nil {
		t.Fatal(err)
	}
	want := "package x\n\nimport (\n\t\"github.com/other/b\"\n\n\t\"example.com/me/a\"\n)\n\nvar _ = a.X\nvar _ = b.Y\n"
	if got.Content != want {
		t.Errorf("content:\n%s\nwant:\n%s", got.Content, want)
	}
}

func TestFormatSyntaxError(t *testing.T) {
	for _, mode := range []Mode{ModeGofmt, ModeGoimports} {
		_, err := Format("x.go", "package x\n\nfunc f() {\n\tx :=\n}\n", mode, "")
		var se *SyntaxError
		if !errors.As(err, &se) {
			t.Fatalf("%s: err = %v, want *SyntaxError", mode, err)
		}
		if se.Line != 5 {
			t.Errorf("%s: line = %d, want 5 (%v)", mode, se.Line, se)
		}
	}
}

func fiveThousandLines() string {
	var b strings.Builder
	b.WriteString("package x\n\nimport \"fmt\"\n\n")
	for i := 0; b.Len() == 0 || strings.Count(b.String(), "\n") < 5000; i++ {
		fmt.Fprintf(&b, "func f%d(a,b int) int {\n\tif a>b { return a }\n\tfmt.Println( a ,b )\n\treturn b\n}\n", i)
	}
	return b.String()
}

func BenchmarkFormat5kLines(b *testing.B) {
	src := fiveThousandLines()
	path := filepath.Join(b.TempDir(), "x.go")
	for _, mode := range []Mode{ModeGofmt, ModeGoimports} {
		b.Run(string(mode), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := Format(path, src, mode, ""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
