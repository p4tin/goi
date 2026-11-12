package rpc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/gopls"
	"github.com/p4tin/goi/internal/lint"
	"github.com/p4tin/goi/internal/lsp"
	"github.com/p4tin/goi/internal/toolbin"
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
	GoiLintFix []gopls.FileEdit `json:"goiLintFix"`
}

// lintSession tracks runs for the open project.
type lintSession struct {
	mu         sync.Mutex
	running    map[string]context.CancelFunc
	installing bool
	major      int
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

// LintService runs golangci-lint for the open project.
type LintService struct {
	app *App
}

func (s *LintService) session() (*session, *lintSession, error) {
	sess, err := s.app.current()
	if err != nil {
		return nil, nil, err
	}
	if sess.lint == nil {
		return nil, nil, errNoProject
	}
	return sess, sess.lint, nil
}

// resolveGolangci finds golangci-lint with the window's settings and SDK.
func (w *window) resolveGolangci() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	opts := toolbin.Options{Setting: w.settings.Effective().Lint.GolangciPath, ToolsDir: w.app.toolsDir()}
	if r, err := w.runner(ctx); err == nil {
		if ge, err := r.GoEnv(ctx); err == nil {
			opts.GOBIN, opts.GOPATH = ge["GOBIN"], ge["GOPATH"]
		}
	}
	return lint.Resolve(opts)
}

// version caches the golangci-lint version per binary.
func (s *LintService) version(ls *lintSession, bin string) (int, string, error) {
	ls.mu.Lock()
	if ls.bin == bin && ls.major > 0 {
		m, v := ls.major, ls.version
		ls.mu.Unlock()
		return m, v, nil
	}
	ls.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	m, v, err := lint.Version(ctx, bin)
	if err != nil {
		return 0, "", err
	}
	ls.mu.Lock()
	ls.bin, ls.major, ls.version = bin, m, v
	ls.mu.Unlock()
	return m, v, nil
}

func (s *LintService) status(ls *lintSession) LintStatus {
	ls.mu.Lock()
	st := LintStatus{Installing: ls.installing, Running: len(ls.running)}
	ls.mu.Unlock()
	bin, err := s.app.mainWindow().resolveGolangci()
	if err != nil {
		st.Message = err.Error()
		return st
	}
	st.Found, st.Bin = true, bin
	if _, v, err := s.version(ls, bin); err == nil {
		st.Version = v
	} else {
		st.Message = err.Error()
	}
	return st
}

func (s *LintService) emitStatus(ls *lintSession) {
	s.app.mainWindow().bus.Emit(events.TopicLintStatus, s.status(ls))
}

// Status reports whether golangci-lint is available.
func (s *LintService) Status() LintStatus {
	_, ls, err := s.session()
	if err != nil {
		return LintStatus{Message: err.Error()}
	}
	return s.status(ls)
}

// Install installs golangci-lint into Goi's tools dir in the background.
func (s *LintService) Install() error {
	_, ls, err := s.session()
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	r, err := s.app.runner(ctx)
	if err != nil {
		cancel()
		ls.mu.Lock()
		ls.installing = false
		ls.mu.Unlock()
		return err
	}
	s.app.appBus.Emit(events.TopicLintStatus, LintStatus{Installing: true, Message: "Installing golangci-lint (go install " + lint.Module + ")…"})
	go func() {
		defer cancel()
		_, err := toolbin.Install(ctx, r.SDK.GoBin, r.Environ(), s.app.toolsDir(), lint.Module, lint.Name)
		ls.mu.Lock()
		ls.installing = false
		ls.bin, ls.major = "", 0
		ls.mu.Unlock()
		if err != nil {
			s.app.log.Warn("install golangci-lint", "err", err)
			s.app.appBus.Emit(events.TopicLintStatus, LintStatus{Message: err.Error()})
			return
		}
		s.emitStatus(ls)
	}()
	return nil
}

// moduleDirFor finds the module directory containing path (nearest go.mod).
func moduleDirFor(path, root string) string {
	d := path
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		d = filepath.Dir(path)
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d || !within(root, parent) && parent != root {
			return ""
		}
		d = parent
	}
}

// LintResult summarizes a run.
type LintResult struct {
	Issues int    `json:"issues"`
	Files  int    `json:"files"`
	Error  string `json:"error"`
}

// Run lints the package containing path ("package") or every module of the
// project ("project"), then emits lint.diagnostics. A newer run of the same
// scope cancels the older one.
func (s *LintService) Run(scope, path string) (LintResult, error) {
	sess, ls, err := s.session()
	if err != nil {
		return LintResult{}, err
	}
	if !s.app.mainWindow().settings.Effective().Lint.Enabled {
		return LintResult{}, errors.New("golangci-lint is disabled in settings (lint.enabled)")
	}
	bin, err := s.app.mainWindow().resolveGolangci()
	if err != nil {
		return LintResult{}, err
	}
	major, _, err := s.version(ls, bin)
	if err != nil {
		return LintResult{}, err
	}
	type job struct {
		dir     string
		targets []string
		covered string
		recurse bool
	}
	var jobs []job
	if scope == "project" {
		for _, m := range sess.manager.Project().Modules {
			jobs = append(jobs, job{dir: m.Dir, targets: []string{"./..."}, covered: m.Dir, recurse: true})
		}
		if len(jobs) == 0 {
			if md := moduleDirFor(sess.root, sess.root); md != "" {
				jobs = append(jobs, job{dir: md, targets: []string{"./..."}, covered: md, recurse: true})
			}
		}
	} else {
		pkgDir := path
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			pkgDir = filepath.Dir(path)
		}
		md := moduleDirFor(pkgDir, sess.root)
		if md == "" {
			return LintResult{}, errors.New("no go.mod above " + pkgDir)
		}
		rel, _ := filepath.Rel(md, pkgDir)
		target := "./" + filepath.ToSlash(rel)
		if rel == "." {
			target = "."
		}
		jobs = append(jobs, job{dir: md, targets: []string{target}, covered: pkgDir})
	}
	if len(jobs) == 0 {
		return LintResult{}, errors.New("no Go module in the project")
	}
	key := scope + ":" + jobs[0].covered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	ls.mu.Lock()
	if prev := ls.running[key]; prev != nil {
		prev()
	}
	ls.running[key] = cancel
	ls.mu.Unlock()
	s.emitStatus(ls)
	defer func() {
		ls.mu.Lock()
		delete(ls.running, key)
		ls.mu.Unlock()
		cancel()
		s.emitStatus(ls)
	}()

	env := os.Environ()
	if r, err := s.app.runner(ctx); err == nil {
		env = r.Environ()
	}
	extra := s.app.mainWindow().settings.Effective().Lint.Args
	ev := LintDiagnosticsEvent{Files: map[string][]lsp.Diagnostic{}, Recursive: scope == "project"}
	res := LintResult{}
	for _, j := range jobs {
		issues, err := lint.Run(ctx, lint.Spec{Bin: bin, Major: major, Dir: j.dir, Env: env, Targets: j.targets, Extra: extra})
		if errors.Is(err, context.Canceled) {
			return LintResult{Error: "cancelled"}, nil
		}
		if err != nil {
			res.Error = err.Error()
			continue
		}
		ev.Dirs = append(ev.Dirs, j.covered)
		for _, is := range issues {
			// Package runs report only that package; keep them, and drop
			// anything outside the project.
			if !within(sess.root, is.Path) && is.Path != sess.root {
				continue
			}
			ev.Files[is.Path] = append(ev.Files[is.Path], lintDiagnostic(is))
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

func lintDiagnostic(is lint.Issue) lsp.Diagnostic {
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
		Source:   "golangci-lint (" + is.Linter + ")",
		Message:  is.Message,
	}
	if len(is.Fix) > 0 {
		edits := make([]lsp.TextEdit, len(is.Fix))
		for i, e := range is.Fix {
			edits[i] = lsp.TextEdit{Range: lsp.Range{Start: lsp.Position{Line: e.Start.Line, Character: e.Start.Character}, End: lsp.Position{Line: e.End.Line, Character: e.End.Character}}, NewText: e.NewText}
		}
		d.Data = LintFix{GoiLintFix: []gopls.FileEdit{{Path: is.Path, Edits: edits}}}
	}
	return d
}
