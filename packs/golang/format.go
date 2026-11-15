package golang

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/packs/golang/gofmt"
)

// formatter is gofmt/goimports as a pack.Formatter.
type formatter struct{}

// Handles reports .go files.
func (formatter) Handles(path string) bool { return strings.HasSuffix(path, ".go") }

// Format runs goimports or gofmt (mode); imports of the module containing
// path are grouped after third-party ones.
func (formatter) Format(_ context.Context, p pack.FormatContext, path, content, mode string) (pack.FormatResult, error) {
	res, err := gofmt.Format(path, content, gofmt.Mode(mode), localPrefix(p, path))
	var se *gofmt.SyntaxError
	if errors.As(err, &se) {
		return pack.FormatResult{Content: content, SyntaxError: &pack.SyntaxError{Line: se.Line, Col: se.Col, Msg: se.Msg}}, nil
	}
	if err != nil {
		return pack.FormatResult{Content: content}, err
	}
	return pack.FormatResult{Content: res.Content, Changed: res.Changed}, nil
}

// localPrefix returns the module path of the project module that contains
// path (longest directory match), or "".
func localPrefix(p pack.FormatContext, path string) string {
	best, bestLen := "", -1
	for _, m := range p.Project.Modules {
		if within(m.Dir, path) && len(m.Dir) > bestLen {
			best, bestLen = m.Path, len(m.Dir)
		}
	}
	return best
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
