package rpc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/langserver"
	"github.com/p4tin/goi/internal/lint"
	"github.com/p4tin/goi/internal/lsp"
	"github.com/p4tin/goi/internal/pack"
)

// LintStatus is the payload of lint.status and the result of Status.
type LintStatus struct {
	Found      bool   `json:"found"`
	Bin        string `json:"bin"`
	Version    string `json:"version"`
	Installing bool   `json:"installing"`
	Running    int    `json:"running"`
	Message    string `json:"message"`
}

// LintDiagnosticsEvent is the payload of lint.diagnostics: the run covered
// Dirs (recursively when Recursive), replacing earlier lint diagnostics of
// files under them.
type LintDiagnosticsEvent struct {
	Dirs      []string                    `json:"dirs"`
	Recursive bool                        `json:"recursive"`
	Files     map[string][]lsp.Diagnostic `json:"files"`
}

// LintFix is the data attached to a fixable lint diagnostic.
type LintFix struct {
	GoiLintFix []langserver.FileEdit `json:"goiLintFix"`
}

// lintSession tracks runs for the open project.
type lintSession struct {
	mu         sync.Mutex
	running    map[string]context.CancelFunc
	installing bool
	version    string
	bin        string
}

func newLintSession() *lintSession { return &lintSession{running: map[string]context.CancelFunc{}} }

func (ls *lintSession) close() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	for _, c := range ls.running {
		c()
	}
}

// LintService runs the pack's linter (Goi: golangci-lint) for the open
// project.
type LintService struct {
	app *App
}

var errNoLinter = errors.New("this app has no linter")

func (s *LintService) session(ctx context.Context) (*session, *lintSession, pack.Linter, error) {
	l := s.app.pack.Linter
	if l == nil {
		return nil, nil, nil, errNoLinter
	}
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if sess.lint == nil {
		return nil, nil, nil, errNoProject
	}
	return sess, sess.lint, l, nil
}

// resolveLinter finds the linter with the window's settings and toolchain.
func (w *window) resolveLinter(l pack.Linter) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return l.Resolve(ctx, w.projectEnv(), w.app.toolsDir())
}

// version caches the linter version per binary.
func (s *LintService) version(ls *lintSession, l pack.Linter, bin string) (string, error) {
	ls.mu.Lock()
	if ls.bin == bin && ls.version != "" {
		v := ls.version
		ls.mu.Unlock()
		return v, nil
	}
	ls.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	v, err := l.Version(ctx, bin)
	if err != nil {
		return "", err
	}
	ls.mu.Lock()
	ls.bin, ls.version = bin, v
	ls.mu.Unlock()
	return v, nil
}

// status reports the linter as resolved for window w.
func (s *LintService) status(w *window, ls *lintSession, l pack.Linter) LintStatus {
	ls.mu.Lock()
	st := LintStatus{Installing: ls.installing, Running: len(ls.running)}
	ls.mu.Unlock()
	bin, err := w.resolveLinter(l)
	if err != nil {
		st.Message = err.Error()
		return st
	}
	st.Found, st.Bin = true, bin
	if v, err := s.version(ls, l, bin); err == nil {
		st.Version = v
	} else {
		st.Message = err.Error()
	}
	return st
}

// emitStatus sends lint.status to window w.
func (s *LintService) emitStatus(w *window, ls *lintSession, l pack.Linter) {
	w.bus.Emit(events.TopicLintStatus, s.status(w, ls, l))
}

// Status reports whether the linter is available.
func (s *LintService) Status(ctx context.Context) LintStatus {
	sess, ls, l, err := s.session(ctx)
	if err != nil {
		return LintStatus{Message: err.Error()}
	}
	return s.status(sess.win, ls, l)
}

// Install installs the linter into the tools dir in the background.
func (s *LintService) Install(ctx context.Context) error {
	sess, ls, l, err := s.session(ctx)
	if err != nil {
		return err
	}
	ls.mu.Lock()
	if ls.installing {
		ls.mu.Unlock()
		return nil
	}
	ls.installing = true
	ls.mu.Unlock()
	ictx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	p := sess.win.projectEnv()
	s.app.appBus.Emit(events.TopicLintStatus, LintStatus{Installing: true, Message: "Installing " + l.Name() + " (" + l.InstallHint() + ")…"})
	go func() {
		defer cancel()
		_, err := l.Install(ictx, p, s.app.toolsDir())
		ls.mu.Lock()
		ls.installing = false
		ls.bin, ls.version = "", ""
		ls.mu.Unlock()
		if err != nil {
			s.app.log.Warn("install linter", "name", l.Name(), "err", err)
			s.app.appBus.Emit(events.TopicLintStatus, LintStatus{Message: err.Error()})
			return
		}
		s.emitStatus(sess.win, ls, l)
	}()
	return nil
}

// LintResult summarizes a run.
type LintResult struct {
	Issues int    `json:"issues"`
	Files  int    `json:"files"`
	Error  string `json:"error"`
}

// Run lints the unit containing path ("package") or the whole project
// ("project"), then emits lint.diagnostics. A newer run of the same scope
// cancels the older one.
func (s *LintService) Run(ctx context.Context, scope, path string) (LintResult, error) {
	sess, ls, l, err := s.session(ctx)
	if err != nil {
		return LintResult{}, err
	}
	w := sess.win
	st := w.settings.Effective()
	if !l.Enabled(st) {
		return LintResult{}, errors.New(l.Name() + " is disabled in settings (lint.enabled)")
	}
	bin, err := w.resolveLinter(l)
	if err != nil {
		return LintResult{}, err
	}
	if _, err := s.version(ls, l, bin); err != nil {
		return LintResult{}, err
	}
	jobs, err := l.Jobs(scope, path, sess.manager.Project())
	if err != nil {
		return LintResult{}, err
	}
	key := scope + ":" + jobs[0].Covered
	rctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	ls.mu.Lock()
	if prev := ls.running[key]; prev != nil {
		prev()
	}
	ls.running[key] = cancel
	ls.mu.Unlock()
	s.emitStatus(w, ls, l)
	defer func() {
		ls.mu.Lock()
		delete(ls.running, key)
		ls.mu.Unlock()
		cancel()
		s.emitStatus(w, ls, l)
	}()

	env := w.toolEnv(rctx)
	ev := LintDiagnosticsEvent{Files: map[string][]lsp.Diagnostic{}, Recursive: scope == "project"}
	res := LintResult{}
	for _, j := range jobs {
		issues, err := l.Run(rctx, bin, j, env, st)
		if errors.Is(err, context.Canceled) {
			return LintResult{Error: "cancelled"}, nil
		}
		if err != nil {
			res.Error = err.Error()
			continue
		}
		ev.Dirs = append(ev.Dirs, j.Covered)
		for _, is := range issues {
			// Package runs report only that package; keep them, and drop
			// anything outside the project.
			if !within(sess.root, is.Path) && is.Path != sess.root {
				continue
			}
			ev.Files[is.Path] = append(ev.Files[is.Path], lintDiagnostic(l.Name(), is))
			res.Issues++
		}
	}
	if ev.Dirs == nil {
		ev.Dirs = []string{}
	}
	res.Files = len(ev.Files)
	sess.win.bus.Emit(events.TopicLintDiagnostics, ev)
	return res, nil
}

func lintDiagnostic(linter string, is lint.Issue) lsp.Diagnostic {
	sev := 2 // warning
	switch strings.ToLower(is.Severity) {
	case "error":
		sev = 1
	case "info":
		sev = 3
	case "hint":
		sev = 4
	}
	d := lsp.Diagnostic{
		Range:    lsp.Range{Start: lsp.Position{Line: is.Start.Line, Character: is.Start.Character}, End: lsp.Position{Line: is.End.Line, Character: is.End.Character}},
		Severity: sev,
		Source:   linter + " (" + is.Linter + ")",
		Message:  is.Message,
	}
	if len(is.Fix) > 0 {
		edits := make([]lsp.TextEdit, len(is.Fix))
		for i, e := range is.Fix {
			edits[i] = lsp.TextEdit{Range: lsp.Range{Start: lsp.Position{Line: e.Start.Line, Character: e.Start.Character}, End: lsp.Position{Line: e.End.Line, Character: e.End.Character}}, NewText: e.NewText}
		}
		d.Data = LintFix{GoiLintFix: []langserver.FileEdit{{Path: is.Path, Edits: edits}}}
	}
	return d
}
