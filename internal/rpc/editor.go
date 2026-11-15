package rpc

import (
	"context"

	"github.com/p4tin/goi/internal/pack"
)

// EditorService provides editor operations backed by the pack's tooling.
type EditorService struct {
	app *App
}

// FormatResult is the outcome of formatting a buffer. A syntax error is
// reported in SyntaxError rather than as an error.
type FormatResult struct {
	Content     string            `json:"content"`
	Changed     bool              `json:"changed"`
	SyntaxError *pack.SyntaxError `json:"syntaxError"`
}

// Format formats content using the calling window's save.format setting.
func (s *EditorService) Format(ctx context.Context, path, content string) (FormatResult, error) {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return FormatResult{Content: content}, err
	}
	return s.FormatWith(ctx, path, content, w.settings.Effective().Save.Format)
}

// FormatWith formats content with an explicit mode (Goi:
// goimports|gofmt|none). Files the pack's formatter doesn't handle are
// returned unchanged.
func (s *EditorService) FormatWith(ctx context.Context, path, content, mode string) (FormatResult, error) {
	f := s.app.pack.Formatter
	if f == nil || !f.Handles(path) {
		return FormatResult{Content: content}, nil
	}
	var fc pack.FormatContext
	if sess, err := s.app.sessionFor(ctx); err == nil {
		fc.Project = sess.manager.Project()
	}
	res, err := f.Format(ctx, fc, path, content, mode)
	if err != nil {
		return FormatResult{Content: content}, err
	}
	return FormatResult{Content: res.Content, Changed: res.Changed, SyntaxError: res.SyntaxError}, nil
}
