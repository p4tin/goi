package format

import (
	"errors"
	"fmt"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"sync"

	"golang.org/x/tools/imports"
)

// Mode selects the formatter applied on save.
type Mode string

const (
	ModeGoimports Mode = "goimports"
	ModeGofmt     Mode = "gofmt"
	ModeNone      Mode = "none"
)

// Result is the formatted source.
type Result struct {
	Content string `json:"content"`
	Changed bool   `json:"changed"`
}

// SyntaxError reports the first parse error in the source. Line and Col
// are 1-based.
type SyntaxError struct {
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Msg  string `json:"msg"`
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg)
}

// imports.LocalPrefix is package-global state in x/tools; serialize access.
var importsMu sync.Mutex

// Format formats Go source. path is used by goimports to resolve the
// enclosing module; localPrefix groups matching imports after third-party
// ones ("" disables). Unparseable source returns a *SyntaxError.
func Format(path, src string, mode Mode, localPrefix string) (Result, error) {
	if mode == ModeNone || mode == "" {
		return Result{Content: src}, nil
	}
	if err := checkSyntax(path, src); err != nil {
		return Result{}, err
	}

	var out []byte
	var err error
	switch mode {
	case ModeGofmt:
		out, err = format.Source([]byte(src))
	case ModeGoimports:
		importsMu.Lock()
		imports.LocalPrefix = localPrefix
		out, err = imports.Process(path, []byte(src), &imports.Options{
			Comments:  true,
			TabIndent: true,
			TabWidth:  8,
		})
		importsMu.Unlock()
	default:
		return Result{}, fmt.Errorf("format: unknown mode %q", mode)
	}
	if err != nil {
		return Result{}, fmt.Errorf("format: %w", err)
	}
	s := string(out)
	return Result{Content: s, Changed: s != src}, nil
}

func checkSyntax(path, src string) error {
	_, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err == nil {
		return nil
	}
	var list scanner.ErrorList
	if errors.As(err, &list) && len(list) > 0 {
		e := list[0]
		return &SyntaxError{Line: e.Pos.Line, Col: e.Pos.Column, Msg: e.Msg}
	}
	return &SyntaxError{Line: 1, Col: 1, Msg: err.Error()}
}
