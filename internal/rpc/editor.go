package rpc

import (
	"errors"
	"strings"

	"github.com/p4tin/goi/internal/format"
)

// EditorService provides editor operations backed by Go tooling.
type EditorService struct {
	app *App
}

// FormatResult is the outcome of formatting a buffer. A syntax error is
// reported in SyntaxError rather than as an error.
type FormatResult struct {
	Content     string              `json:"content"`
	Changed     bool                `json:"changed"`
	SyntaxError *format.SyntaxError `json:"syntaxError"`
}

// Format formats content using the save.format setting.
func (s *EditorService) Format(path, content string) (FormatResult, error) {
	return s.FormatWith(path, content, s.app.mainWindow().settings.Effective().Save.Format)
}

// FormatWith formats content with an explicit mode (goimports|gofmt|none).
// Non-Go files are returned unchanged.
func (s *EditorService) FormatWith(path, content, mode string) (FormatResult, error) {
	if !strings.HasSuffix(path, ".go") {
		return FormatResult{Content: content}, nil
	}
	res, err := format.Format(path, content, format.Mode(mode), s.localPrefix(path))
	var se *format.SyntaxError
	if errors.As(err, &se) {
		return FormatResult{Content: content, SyntaxError: se}, nil
	}
	if err != nil {
		return FormatResult{Content: content}, err
	}
	return FormatResult{Content: res.Content, Changed: res.Changed}, nil
}

// localPrefix returns the module path of the open project's module that
// contains path (longest directory match), or "".
func (s *EditorService) localPrefix(path string) string {
	sess, err := s.app.current()
	if err != nil {
		return ""
	}
	best, bestLen := "", -1
	for _, m := range sess.manager.Project().Modules {
		if within(m.Dir, path) && len(m.Dir) > bestLen {
			best, bestLen = m.Path, len(m.Dir)
		}
	}
	return best
}
