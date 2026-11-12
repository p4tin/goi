package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/gopls"
	"github.com/p4tin/goi/internal/lsp"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/vfs"
)

// Lang states beyond gopls.State.
const (
	LangMissing    = "missing"
	LangDisabled   = "disabled"
	LangInstalling = "installing"
)

// LangStatus is the payload of lang.status.
type LangStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
	Bin     string `json:"bin"`
}

// DiagnosticsEvent is the payload of lang.diagnostics.
type DiagnosticsEvent struct {
	Path  string           `json:"path"`
	Items []lsp.Diagnostic `json:"items"`
}

// ApplyEditEvent is the payload of lang.applyEdit.
type ApplyEditEvent struct {
	Label string           `json:"label"`
	Edits []gopls.FileEdit `json:"edits"`
}

var errLangNotRunning = errors.New("gopls is not running")

// langSession owns the project's gopls server.
type langSession struct {
	app      *App
	win      *window
	root     string // as opened (may contain symlinks)
	realRoot string // symlinks resolved, as gopls may report paths

	mu         sync.Mutex
	srv        *gopls.Server
	status     LangStatus
	cfgKey     string
	installing bool
	closed     bool
	cancels    []func()
}

func startLang(win *window, root string, w vfs.Watcher) *langSession {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		real = root
	}
	ls := &langSession{app: win.app, win: win, root: root, realRoot: real}
	ls.cancels = append(ls.cancels,
		w.Subscribe(ls.onFiles),
		win.settings.Subscribe(func(settings.Settings) { ls.reconfigure() }),
	)
	go ls.start()
	return ls
}

func (ls *langSession) setStatus(st LangStatus) {
	ls.mu.Lock()
	ls.status = st
	ls.mu.Unlock()
	ls.win.bus.Emit(events.TopicLangStatus, st)
}

func (ls *langSession) currentStatus() LangStatus {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.status
}

func (ls *langSession) server() *gopls.Server {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.srv
}

// configKey summarizes the settings that require a gopls restart.
func configKey(s settings.Settings) string {
	b, _ := json.Marshal(struct {
		Go    settings.Go
		Gopls settings.Gopls
	}{s.Go, s.Gopls})
	return string(b)
}

type langConfig struct {
	bin        string
	env        []string
	buildFlags []string
	settings   map[string]any
}

// resolveConfig computes the gopls binary and environment from settings.
func (ls *langSession) resolveConfig() (langConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s := ls.win.settings.Effective()
	opts := gopls.ResolveOptions{Setting: s.Gopls.Path, ToolsDir: ls.app.toolsDir()}
	var env []string
	if r, err := ls.win.runner(ctx); err == nil {
		env = r.Environ()
		if ge, err := r.GoEnv(ctx); err == nil {
			opts.GOBIN, opts.GOPATH = ge["GOBIN"], ge["GOPATH"]
		}
	} else {
		env = os.Environ()
	}
	bin, err := gopls.Resolve(opts)
	if err != nil {
		return langConfig{}, err
	}
	cfg := map[string]any{
		"usePlaceholders":    s.Gopls.UsePlaceholders,
		"staticcheck":        s.Gopls.Staticcheck,
		"completeUnimported": true,
		"deepCompletion":     true,
		"matcher":            "Fuzzy",
		"linksInHover":       true,
		"hoverKind":          "FullDocumentation",
	}
	for k, v := range s.Gopls.Settings {
		cfg[k] = v
	}
	var flags []string
	if len(s.Go.Tags) > 0 {
		flags = append(flags, "-tags="+strings.Join(s.Go.Tags, ","))
	}
	switch s.Go.Vendor {
	case "on":
		flags = append(flags, "-mod=vendor")
	case "off":
		flags = append(flags, "-mod=mod")
	}
	return langConfig{bin: bin, env: env, buildFlags: flags, settings: cfg}, nil
}

func (ls *langSession) start() {
	s := ls.win.settings.Effective()
	ls.mu.Lock()
	ls.cfgKey = configKey(s)
	ls.mu.Unlock()
	if !s.Gopls.Enabled {
		ls.setStatus(LangStatus{State: LangDisabled, Message: "gopls is disabled in settings"})
		return
	}
	cfg, err := ls.resolveConfig()
	if errors.Is(err, gopls.ErrNotFound) {
		ls.setStatus(LangStatus{State: LangMissing, Message: err.Error()})
		return
	}
	if err != nil {
		ls.setStatus(LangStatus{State: string(gopls.StateError), Message: err.Error()})
		return
	}
	srv := gopls.New(gopls.Config{
		Bin:        cfg.bin,
		Root:       ls.root,
		Env:        cfg.env,
		BuildFlags: cfg.buildFlags,
		Settings:   cfg.settings,
		Log:        ls.app.log,
		OnStatus: func(st gopls.Status) {
			ls.setStatus(LangStatus{State: string(st.State), Message: st.Message, Bin: cfg.bin})
		},
		OnDiagnostics: func(path string, d []lsp.Diagnostic) {
			ls.win.bus.Emit(events.TopicLangDiagnostics, DiagnosticsEvent{Path: ls.toEditor(path), Items: d})
		},
		OnApplyEdit: func(label string, edits []gopls.FileEdit) bool {
			ls.win.bus.Emit(events.TopicLangApplyEdit, ApplyEditEvent{Label: label, Edits: ls.mapEdits(edits)})
			return true
		},
	})
	ls.mu.Lock()
	if ls.closed {
		ls.mu.Unlock()
		return
	}
	ls.srv = srv
	ls.mu.Unlock()
	ls.app.log.Info("gopls starting", "bin", cfg.bin, "root", ls.root)
	srv.Start()
}

// reconfigure restarts gopls when relevant settings changed.
func (ls *langSession) reconfigure() {
	s := ls.win.settings.Effective()
	key := configKey(s)
	ls.mu.Lock()
	if key == ls.cfgKey {
		ls.mu.Unlock()
		return
	}
	ls.cfgKey = key
	srv := ls.srv
	ls.mu.Unlock()
	if srv == nil || !s.Gopls.Enabled {
		if srv != nil {
			srv.Close()
			ls.mu.Lock()
			ls.srv = nil
			ls.mu.Unlock()
		}
		ls.start()
		return
	}
	cfg, err := ls.resolveConfig()
	if err != nil {
		srv.Close()
		ls.mu.Lock()
		ls.srv = nil
		ls.mu.Unlock()
		ls.start()
		return
	}
	srv.Reconfigure(func(c *gopls.Config) {
		c.Bin, c.Env, c.BuildFlags, c.Settings = cfg.bin, cfg.env, cfg.buildFlags, cfg.settings
	})
}

func (ls *langSession) restart() {
	if srv := ls.server(); srv != nil {
		srv.Restart()
		return
	}
	ls.start()
}

// install runs `go install gopls` into the tools dir, then starts gopls.
func (ls *langSession) install() error {
	ls.mu.Lock()
	if ls.installing {
		ls.mu.Unlock()
		return nil
	}
	ls.installing = true
	ls.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	r, err := ls.win.runner(ctx)
	if err != nil {
		cancel()
		ls.mu.Lock()
		ls.installing = false
		ls.mu.Unlock()
		return err
	}
	ls.setStatus(LangStatus{State: LangInstalling, Message: "Installing gopls (go install " + gopls.Module + ")…"})
	go func() {
		defer cancel()
		bin, err := gopls.Install(ctx, r.SDK.GoBin, r.Environ(), ls.app.toolsDir())
		ls.mu.Lock()
		ls.installing = false
		ls.mu.Unlock()
		if err != nil {
			ls.app.log.Warn("gopls install", "err", err)
			ls.setStatus(LangStatus{State: LangMissing, Message: err.Error()})
			ls.win.bus.Emit(events.TopicNotify, events.Notification{ID: "gopls.install", Level: "error", Title: "Could not install gopls", Body: err.Error()})
			return
		}
		ls.app.log.Info("gopls installed", "bin", bin)
		ls.win.bus.Emit(events.TopicNotify, events.Notification{ID: "gopls.install", Level: "info", Title: "gopls installed", Body: bin})
		if srv := ls.server(); srv != nil {
			srv.Close()
		}
		ls.mu.Lock()
		ls.srv = nil
		ls.mu.Unlock()
		ls.start()
	}()
	return nil
}

func (ls *langSession) close() {
	ls.mu.Lock()
	ls.closed = true
	srv := ls.srv
	ls.srv = nil
	cancels := ls.cancels
	ls.cancels = nil
	ls.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	if srv != nil {
		srv.Close()
	}
}

// onFiles forwards Go-relevant disk changes to gopls.
func (ls *langSession) onFiles(evs []vfs.Event) {
	srv := ls.server()
	if srv == nil {
		return
	}
	var out []lsp.FileEvent
	for _, e := range evs {
		if e.IsDir || !watchedByGopls(e.Path) {
			continue
		}
		t := lsp.FileChanged
		switch e.Op {
		case vfs.OpCreate:
			t = lsp.FileCreated
		case vfs.OpRemove, vfs.OpRename:
			t = lsp.FileDeleted
		}
		out = append(out, lsp.FileEvent{URI: gopls.PathToURI(e.Path), Type: t})
	}
	srv.WatchedFilesChanged(out)
}

func watchedByGopls(p string) bool {
	base := filepath.Base(p)
	return strings.HasSuffix(base, ".go") || base == "go.mod" || base == "go.sum" || base == "go.work" || base == "go.work.sum"
}

// toEditor maps a path reported by gopls (possibly symlink-resolved) back
// under the root the editor uses.
func (ls *langSession) toEditor(p string) string {
	if ls.realRoot == ls.root {
		return p
	}
	if p == ls.realRoot {
		return ls.root
	}
	if strings.HasPrefix(p, ls.realRoot+string(filepath.Separator)) {
		return ls.root + p[len(ls.realRoot):]
	}
	return p
}

func (ls *langSession) mapLocs(locs []gopls.Location) []gopls.Location {
	if locs == nil {
		return []gopls.Location{}
	}
	for i := range locs {
		locs[i].Path = ls.toEditor(locs[i].Path)
	}
	return locs
}

func (ls *langSession) mapEdits(edits []gopls.FileEdit) []gopls.FileEdit {
	if edits == nil {
		return []gopls.FileEdit{}
	}
	for i := range edits {
		edits[i].Path = ls.toEditor(edits[i].Path)
	}
	return edits
}

// toolsDir is where Goi installs gopls and other Go tools.
func (a *App) toolsDir() string {
	return filepath.Join(a.configDir, "tools", "bin")
}

// LangService exposes gopls-backed code intelligence for the open project.
type LangService struct {
	app *App
}

// HoverResult is rendered documentation for a position.
type HoverResult struct {
	Markdown string     `json:"markdown"`
	Range    *lsp.Range `json:"range"`
}

func (s *LangService) lang() (*langSession, error) {
	sess, err := s.app.current()
	if err != nil {
		return nil, err
	}
	if sess.lang == nil {
		return nil, errLangNotRunning
	}
	return sess.lang, nil
}

func (s *LangService) srv() (*langSession, *gopls.Server, error) {
	ls, err := s.lang()
	if err != nil {
		return nil, nil, err
	}
	srv := ls.server()
	if srv == nil {
		return ls, nil, errLangNotRunning
	}
	return ls, srv, nil
}

func reqCtx(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func pos(line, char int) lsp.Position { return lsp.Position{Line: line, Character: char} }

// Status returns the gopls status.
func (s *LangService) Status() LangStatus {
	ls, err := s.lang()
	if err != nil {
		return LangStatus{State: string(gopls.StateStopped), Message: "No project open"}
	}
	return ls.currentStatus()
}

// Restart restarts gopls.
func (s *LangService) Restart() error {
	ls, err := s.lang()
	if err != nil {
		return err
	}
	ls.restart()
	return nil
}

// InstallGopls installs gopls into Goi's tools directory and starts it.
func (s *LangService) InstallGopls() error {
	ls, err := s.lang()
	if err != nil {
		return err
	}
	return ls.install()
}

// DidOpen tracks an editor buffer.
func (s *LangService) DidOpen(path string, version int, text string) {
	if _, srv, err := s.srv(); err == nil && gopls.LanguageID(path) != "" {
		srv.Open(path, version, text)
	}
}

// DidChange sends a buffer's full text.
func (s *LangService) DidChange(path string, version int, text string) {
	if _, srv, err := s.srv(); err == nil && gopls.LanguageID(path) != "" {
		srv.Change(path, version, text)
	}
}

// DidSave notifies gopls that a buffer was saved.
func (s *LangService) DidSave(path string) {
	if _, srv, err := s.srv(); err == nil {
		srv.Save(path)
	}
}

// DidClose stops tracking a buffer.
func (s *LangService) DidClose(path string) {
	if _, srv, err := s.srv(); err == nil {
		srv.CloseDoc(path)
	}
}

// Completion returns completion candidates.
func (s *LangService) Completion(path string, line, char int) (lsp.CompletionList, error) {
	_, srv, err := s.srv()
	if err != nil {
		return lsp.CompletionList{Items: []lsp.CompletionItem{}}, err
	}
	ctx, cancel := reqCtx(10 * time.Second)
	defer cancel()
	return srv.Completion(ctx, path, pos(line, char))
}

// Hover returns documentation for a position (empty Markdown if none).
func (s *LangService) Hover(path string, line, char int) (HoverResult, error) {
	_, srv, err := s.srv()
	if err != nil {
		return HoverResult{}, err
	}
	ctx, cancel := reqCtx(10 * time.Second)
	defer cancel()
	h, err := srv.Hover(ctx, path, pos(line, char))
	if err != nil || h == nil {
		return HoverResult{}, err
	}
	return HoverResult{Markdown: string(h.Contents), Range: h.Range}, nil
}

// SignatureHelp returns the signature of the enclosing call (nil if none).
func (s *LangService) SignatureHelp(path string, line, char int) (*lsp.SignatureHelp, error) {
	_, srv, err := s.srv()
	if err != nil {
		return nil, err
	}
	ctx, cancel := reqCtx(10 * time.Second)
	defer cancel()
	return srv.SignatureHelp(ctx, path, pos(line, char))
}

type locFunc func(*gopls.Server, context.Context, string, lsp.Position) ([]gopls.Location, error)

func (s *LangService) locations(f locFunc, path string, line, char int, timeout time.Duration) ([]gopls.Location, error) {
	ls, srv, err := s.srv()
	if err != nil {
		return []gopls.Location{}, err
	}
	ctx, cancel := reqCtx(timeout)
	defer cancel()
	locs, err := f(srv, ctx, path, pos(line, char))
	return ls.mapLocs(locs), err
}

// Definition returns the declaration(s) of the symbol at a position.
func (s *LangService) Definition(path string, line, char int) ([]gopls.Location, error) {
	return s.locations((*gopls.Server).Definition, path, line, char, 15*time.Second)
}

// TypeDefinition returns the declaration of the symbol's type.
func (s *LangService) TypeDefinition(path string, line, char int) ([]gopls.Location, error) {
	return s.locations((*gopls.Server).TypeDefinition, path, line, char, 15*time.Second)
}

// Implementation returns implementations or implemented interfaces.
func (s *LangService) Implementation(path string, line, char int) ([]gopls.Location, error) {
	return s.locations((*gopls.Server).Implementation, path, line, char, 30*time.Second)
}

// References returns the usages of the symbol at a position.
func (s *LangService) References(path string, line, char int) ([]gopls.Location, error) {
	return s.locations((*gopls.Server).References, path, line, char, 60*time.Second)
}

// DocumentHighlight returns the occurrences of the symbol in its file.
func (s *LangService) DocumentHighlight(path string, line, char int) ([]lsp.DocumentHighlight, error) {
	_, srv, err := s.srv()
	if err != nil {
		return []lsp.DocumentHighlight{}, err
	}
	ctx, cancel := reqCtx(5 * time.Second)
	defer cancel()
	return srv.DocumentHighlight(ctx, path, pos(line, char))
}

// DocumentSymbols returns the symbol tree of a file.
func (s *LangService) DocumentSymbols(path string) ([]lsp.DocumentSymbol, error) {
	_, srv, err := s.srv()
	if err != nil {
		return []lsp.DocumentSymbol{}, err
	}
	ctx, cancel := reqCtx(10 * time.Second)
	defer cancel()
	return srv.DocumentSymbols(ctx, path)
}

// WorkspaceSymbols searches symbols across the project.
func (s *LangService) WorkspaceSymbols(query string) ([]gopls.Symbol, error) {
	ls, srv, err := s.srv()
	if err != nil {
		return []gopls.Symbol{}, err
	}
	ctx, cancel := reqCtx(10 * time.Second)
	defer cancel()
	syms, err := srv.WorkspaceSymbols(ctx, query)
	for i := range syms {
		syms[i].Location.Path = ls.toEditor(syms[i].Location.Path)
	}
	return syms, err
}

// PrepareRename checks that the symbol at a position can be renamed.
func (s *LangService) PrepareRename(path string, line, char int) (lsp.PrepareRenameResult, error) {
	_, srv, err := s.srv()
	if err != nil {
		return lsp.PrepareRenameResult{}, err
	}
	ctx, cancel := reqCtx(15 * time.Second)
	defer cancel()
	return srv.PrepareRename(ctx, path, pos(line, char))
}

// Rename computes the edits that rename the symbol at a position.
func (s *LangService) Rename(path string, line, char int, newName string) ([]gopls.FileEdit, error) {
	ls, srv, err := s.srv()
	if err != nil {
		return []gopls.FileEdit{}, err
	}
	ctx, cancel := reqCtx(60 * time.Second)
	defer cancel()
	edits, err := srv.Rename(ctx, path, pos(line, char), newName)
	return ls.mapEdits(edits), err
}

// CodeActions lists quick-fixes, refactorings and source actions.
func (s *LangService) CodeActions(path string, rng lsp.Range, diags []lsp.Diagnostic) ([]lsp.CodeAction, error) {
	_, srv, err := s.srv()
	if err != nil {
		return []lsp.CodeAction{}, err
	}
	ctx, cancel := reqCtx(15 * time.Second)
	defer cancel()
	return srv.CodeActions(ctx, path, rng, diags)
}

// RunCodeAction returns a code action's edits, resolving them first if
// needed, and runs its command (whose edits arrive as lang.applyEdit).
func (s *LangService) RunCodeAction(action lsp.CodeAction) ([]gopls.FileEdit, error) {
	ls, srv, err := s.srv()
	if err != nil {
		return []gopls.FileEdit{}, err
	}
	ctx, cancel := reqCtx(60 * time.Second)
	defer cancel()
	if action.Edit == nil && action.Command == nil && action.Data != nil {
		resolved, err := srv.ResolveCodeAction(ctx, action)
		if err != nil {
			return []gopls.FileEdit{}, err
		}
		action = resolved
	}
	edits := []gopls.FileEdit{}
	if action.Edit != nil {
		edits = ls.mapEdits(gopls.FileEdits(*action.Edit))
	}
	if action.Command != nil {
		if err := srv.ExecuteCommand(ctx, *action.Command); err != nil {
			return edits, err
		}
	}
	return edits, nil
}

// ApplyFileEdits applies edits to files on disk (files not open in the
// editor), preserving their line endings and BOM.
func (s *LangService) ApplyFileEdits(edits []gopls.FileEdit) error {
	var errs []error
	for _, fe := range edits {
		meta, text, err := vfs.ReadText(fe.Path, vfs.Limits{})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if meta.Binary {
			errs = append(errs, fmt.Errorf("%s is binary", fe.Path))
			continue
		}
		out, err := gopls.ApplyEdits(text, fe.Edits)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", fe.Path, err))
			continue
		}
		if out == text {
			continue
		}
		if _, err := vfs.WriteText(fe.Path, out, meta); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
