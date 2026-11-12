package rpc

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEditorFormat(t *testing.T) {
	root := module(t)
	a, _ := started(t, root)
	ed := a.editorSvc
	path := filepath.Join(root, "x.go")

	res, err := ed.Format(context.Background(), path, "package m\nfunc  f( ) {  }\n")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Content != "package m\n\nfunc f() {}\n" || res.SyntaxError != nil {
		t.Errorf("format = %+v", res)
	}

	res, err = ed.Format(context.Background(), path, "package m\n\nfunc f() {\n\tx :=\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if res.SyntaxError == nil || res.Changed || res.SyntaxError.Line != 5 {
		t.Errorf("syntax error result = %+v", res)
	}

	if res, _ := ed.Format(context.Background(), filepath.Join(root, "notes.txt"), "a  b"); res.Changed || res.Content != "a  b" {
		t.Errorf("non-go = %+v", res)
	}

	if err := a.settingsSvc.Set("project", "save.format", "none"); err != nil {
		t.Fatal(err)
	}
	if res, _ := ed.Format(context.Background(), path, "package m\nfunc  f( ) {  }\n"); res.Changed {
		t.Errorf("none mode changed: %+v", res)
	}
	if res, _ := ed.FormatWith(context.Background(), path, "package m\nfunc  f( ) {  }\n", "gofmt"); !res.Changed {
		t.Errorf("explicit gofmt did not change: %+v", res)
	}
}

func TestEditorFormatLocalPrefix(t *testing.T) {
	root := module(t) // module example.com/m
	a, _ := started(t, root)
	src := "package m\n\nimport (\n\t\"example.com/m/pkg/util\"\n\t\"github.com/other/b\"\n)\n\nvar _ = util.X\nvar _ = b.Y\n"
	res, err := a.editorSvc.FormatWith(context.Background(), filepath.Join(root, "x.go"), src, "goimports")
	if err != nil {
		t.Fatal(err)
	}
	want := "package m\n\nimport (\n\t\"github.com/other/b\"\n\n\t\"example.com/m/pkg/util\"\n)\n\nvar _ = util.X\nvar _ = b.Y\n"
	if res.Content != want {
		t.Errorf("content:\n%s\nwant:\n%s", res.Content, want)
	}
}
