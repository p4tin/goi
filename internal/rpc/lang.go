package rpc

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/langserver"
	"github.com/p4tin/goi/internal/lsp"
	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/toolbin"
	"github.com/p4tin/goi/internal/vfs"
)

// Lang states beyond langserver.State.
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
	Label string                `json:"label"`
	Edits []langserver.FileEdit `json:"edits"`
}

var errLangNotRunning = errors.New("the language server is not running")

// langSession owns the project's language server.
type langSession struct {
	app      *App
	win      *window
	root     string // as opened (may contain symlinks)
	realRoot string // symlinks resolved, as servers may report paths

	mu         sync.Mutex
	srv        *langserver.Server
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

func (ls *langSession) server() *langserver.Server {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.srv
}

// spec is the pack's language server (nil: none).
func (ls *langSession) spec() pack.LanguageServer { return ls.app.pack.Server }

type langConfig struct {
	cmd      pack.ServerCommand
	env      []string
	settings map[string]any
}

// resolveConfig locates the server and computes its environment and
// settings.
func (ls *langSession) resolveConfig() (langConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sp := ls.spec()
	p := ls.win.projectEnv()
	env := ls.win.toolEnv(ctx)
	cmd, err := sp.Command(ctx, p, env, ls.app.toolsDir())
	if err != nil {
		return langConfig{}, err
	}
	return langConfig{cmd: cmd, env: env, settings: sp.Settings(p)}, nil
}

func (ls *langSession) start() {
	sp := ls.spec()
	if sp == nil {
		ls.setStatus(LangStatus{State: LangDisabled, Message: "No language server"})
		return
	}
	s := ls.win.settings.Effective()
	ls.mu.Lock()
	ls.cfgKey = sp.RestartKey(s)
	ls.mu.Unlock()
	if !sp.Enabled(s) {
		ls.setStatus(LangStatus{State: LangDisabled, Message: sp.Name() + " is disabled in settings"})
		return
	}
	cfg, err := ls.resolveConfig()
	if errors.Is(err, toolbin.ErrNotFound) {
		ls.setStatus(LangStatus{State: LangMissing, Message: err.Error()})
		return
	}
	if err != nil {
		ls.setStatus(LangStatus{State: string(langserver.StateError), Message: err.Error()})
		return
	}
	srv := langserver.New(langserver.Config{
		Name:       sp.Name(),
		Bin:        cfg.cmd.Bin,
		Args:       cfg.cmd.Args,
		Root:       ls.root,
		Env:        cfg.env,
		Settings:   cfg.settings,
		Section:    sp.Section,
		LanguageID: sp.LanguageID,
		Log:        ls.app.log,
		OnStatus: func(st langserver.Status) {
			ls.setStatus(LangStatus{State: string(st.State), Message: st.Message, Bin: cfg.cmd.Bin})
		},
		OnDiagnostics: func(path string, d []lsp.Diagnostic) {
			ls.win.bus.Emit(events.TopicLangDiagnostics, DiagnosticsEvent{Path: ls.toEditor(path), Items: d})
		},
		OnApplyEdit: func(label string, edits []langserver.FileEdit) bool {
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
	ls.app.log.Info("language server starting", "name", sp.Name(), "bin", cfg.cmd.Bin, "root", ls.root)
	srv.Start()
}

// reconfigure restarts the server when relevant settings changed.
func (ls *langSession) reconfigure() {
	sp := ls.spec()
	if sp == nil {
		return
	}
	s := ls.win.settings.Effective()
	key := sp.RestartKey(s)
	ls.mu.Lock()
	if key == ls.cfgKey {
		ls.mu.Unlock()
		return
	}
	ls.cfgKey = key
	srv := ls.srv
	ls.mu.Unlock()
	if srv == nil || !sp.Enabled(s) {
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
	srv.Reconfigure(func(c *langserver.Config) {
		c.Bin, c.Args, c.Env, c.Settings = cfg.cmd.Bin, cfg.cmd.Args, cfg.env, cfg.settings
	})
}

func (ls *langSession) restart() {
	if srv := ls.server(); srv != nil {
		srv.Restart()
		return
	}
	ls.start()
}

// install installs the server into the tools dir, then starts it.
func (ls *langSession) install() error {
	sp := ls.spec()
	if sp == nil {
		return errors.New("no language server")
	}
	ls.mu.Lock()
	if ls.installing {
		ls.mu.Unlock()
		return nil
	}
	ls.installing = true
	ls.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	p := ls.win.projectEnv()
	env := ls.win.toolEnv(ctx)
	ls.setStatus(LangStatus{State: LangInstalling, Message: "Installing " + sp.Name() + " (" + sp.InstallHint() + ")…"})
	go func() {
		defer cancel()
		bin, err := sp.Install(ctx, p, env, ls.app.toolsDir())
		ls.mu.Lock()
		ls.installing = false
		ls.mu.Unlock()
		id := "lang.install"
		if err != nil {
			ls.app.log.Warn("language server install", "name", sp.Name(), "err", err)
			ls.setStatus(LangStatus{State: LangMissing, Message: err.Error()})
			ls.win.bus.Emit(events.TopicNotify, events.Notification{ID: id, Level: "error", Title: "Could not install " + sp.Name(), Body: err.Error()})
			return
		}
		ls.app.log.Info("language server installed", "name", sp.Name(), "bin", bin)
		ls.win.bus.Emit(events.TopicNotify, events.Notification{ID: id, Level: "info", Title: sp.Name() + " installed", Body: bin})
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

// onFiles forwards Go-relevant disk changes to langserver.
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
		out = append(out, lsp.FileEvent{URI: langserver.PathToURI(e.Path), Type: t})
	}
	srv.WatchedFilesChanged(out)
}

func watchedByGopls(p string) bool {
	base := filepath.Base(p)
	return strings.HasSuffix(base, ".go") || base == "go.mod" || base == "go.sum" || base == "go.work" || base == "go.work.sum"
}

// toEditor maps a path reported by the server (possibly symlink-resolved) back
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

func (ls *langSession) mapLocs(locs []langserver.Location) []langserver.Location {
	if locs == nil {
		return []langserver.Location{}
	}
	for i := range locs {
		locs[i].Path = ls.toEditor(locs[i].Path)
	}
	return locs
}

func (ls *langSession) mapEdits(edits []langserver.FileEdit) []langserver.FileEdit {
	if edits == nil {
		return []langserver.FileEdit{}
	}
	for i := range edits {
		edits[i].Path = ls.toEditor(edits[i].Path)
	}
	return edits
}

// toolsDir is where the app installs its tools (language server, debugger, linter).
func (a *App) toolsDir() string {
	return filepath.Join(a.configDir, "tools", "bin")
}

// LangService exposes language-server code intelligence for the open project.
type LangService struct {
	app *App
}

// HoverResult is rendered documentation for a position.
type HoverResult struct {
	Markdown string     `json:"markdown"`
	Range    *lsp.Range `json:"range"`
}

func (s *LangService) lang(ctx context.Context) (*langSession, error) {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return nil, err
	}
	if sess.lang == nil {
		return nil, errLangNotRunning
	}
	return sess.lang, nil
}

func (s *LangService) srv(ctx context.Context) (*langSession, *langserver.Server, error) {
	ls, err := s.lang(ctx)
	if err != nil {
		return nil, nil, err
	}
	srv := ls.server()
	if srv == nil {
		return ls, nil, errLangNotRunning
	}
	return ls, srv, nil
}

// reqCtx bounds a request made during a bound call; it ends with the call.
func reqCtx(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, d)
}

func pos(line, char int) lsp.Position { return lsp.Position{Line: line, Character: char} }

// Status returns the language server status.
func (s *LangService) Status(ctx context.Context) LangStatus {
	ls, err := s.lang(ctx)
	if err != nil {
		return LangStatus{State: string(langserver.StateStopped), Message: "No project open"}
	}
	return ls.currentStatus()
}

// Restart restarts langserver.
func (s *LangService) Restart(ctx context.Context) error {
	ls, err := s.lang(ctx)
	if err != nil {
		return err
	}
	ls.restart()
	return nil
}

// InstallServer installs the language server into the tools directory and starts it.
func (s *LangService) InstallServer(ctx context.Context) error {
	ls, err := s.lang(ctx)
	if err != nil {
		return err
	}
	return ls.install()
}

// DidOpen tracks an editor buffer.
func (s *LangService) DidOpen(ctx context.Context, path string, version int, text string) {
	if _, srv, err := s.srv(ctx); err == nil && srv.LanguageID(path) != "" {
		srv.Open(path, version, text)
	}
}

// DidChange sends a buffer's full text.
func (s *LangService) DidChange(ctx context.Context, path string, version int, text string) {
	if _, srv, err := s.srv(ctx); err == nil && srv.LanguageID(path) != "" {
		srv.Change(path, version, text)
	}
}

// DidSave notifies the server that a buffer was saved.
func (s *LangService) DidSave(ctx context.Context, path string) {
	if _, srv, err := s.srv(ctx); err == nil {
		srv.Save(path)
	}
}

// DidClose stops tracking a buffer.
func (s *LangService) DidClose(ctx context.Context, path string) {
	if _, srv, err := s.srv(ctx); err == nil {
		srv.CloseDoc(path)
	}
}

// Completion returns completion candidates.
func (s *LangService) Completion(ctx context.Context, path string, line, char int) (lsp.CompletionList, error) {
	_, srv, err := s.srv(ctx)
	if err != nil {
		return lsp.CompletionList{Items: []lsp.CompletionItem{}}, err
	}
	rctx, cancel := reqCtx(ctx, 10*time.Second)
	defer cancel()
	return srv.Completion(rctx, path, pos(line, char))
}

// Hover returns documentation for a position (empty Markdown if none).
func (s *LangService) Hover(ctx context.Context, path string, line, char int) (HoverResult, error) {
	_, srv, err := s.srv(ctx)
	if err != nil {
		return HoverResult{}, err
	}
	rctx, cancel := reqCtx(ctx, 10*time.Second)
	defer cancel()
	h, err := srv.Hover(rctx, path, pos(line, char))
	if err != nil || h == nil {
		return HoverResult{}, err
	}
	return HoverResult{Markdown: string(h.Contents), Range: h.Range}, nil
}

// SignatureHelp returns the signature of the enclosing call (nil if none).
func (s *LangService) SignatureHelp(ctx context.Context, path string, line, char int) (*lsp.SignatureHelp, error) {
	_, srv, err := s.srv(ctx)
	if err != nil {
		return nil, err
	}
	rctx, cancel := reqCtx(ctx, 10*time.Second)
	defer cancel()
	return srv.SignatureHelp(rctx, path, pos(line, char))
}

type locFunc func(*langserver.Server, context.Context, string, lsp.Position) ([]langserver.Location, error)

func (s *LangService) locations(ctx context.Context, f locFunc, path string, line, char int, timeout time.Duration) ([]langserver.Location, error) {
	ls, srv, err := s.srv(ctx)
	if err != nil {
		return []langserver.Location{}, err
	}
	rctx, cancel := reqCtx(ctx, timeout)
	defer cancel()
	locs, err := f(srv, rctx, path, pos(line, char))
	return ls.mapLocs(locs), err
}

// Definition returns the declaration(s) of the symbol at a position.
func (s *LangService) Definition(ctx context.Context, path string, line, char int) ([]langserver.Location, error) {
	return s.locations(ctx, (*langserver.Server).Definition, path, line, char, 15*time.Second)
}

// TypeDefinition returns the declaration of the symbol's type.
func (s *LangService) TypeDefinition(ctx context.Context, path string, line, char int) ([]langserver.Location, error) {
	return s.locations(ctx, (*langserver.Server).TypeDefinition, path, line, char, 15*time.Second)
}

// Implementation returns implementations or implemented interfaces.
func (s *LangService) Implementation(ctx context.Context, path string, line, char int) ([]langserver.Location, error) {
	return s.locations(ctx, (*langserver.Server).Implementation, path, line, char, 30*time.Second)
}

// References returns the usages of the symbol at a position.
func (s *LangService) References(ctx context.Context, path string, line, char int) ([]langserver.Location, error) {
	return s.locations(ctx, (*langserver.Server).References, path, line, char, 60*time.Second)
}

// DocumentHighlight returns the occurrences of the symbol in its file.
func (s *LangService) DocumentHighlight(ctx context.Context, path string, line, char int) ([]lsp.DocumentHighlight, error) {
	_, srv, err := s.srv(ctx)
	if err != nil {
		return []lsp.DocumentHighlight{}, err
	}
	rctx, cancel := reqCtx(ctx, 5*time.Second)
	defer cancel()
	return srv.DocumentHighlight(rctx, path, pos(line, char))
}

// DocumentSymbols returns the symbol tree of a file.
func (s *LangService) DocumentSymbols(ctx context.Context, path string) ([]lsp.DocumentSymbol, error) {
	_, srv, err := s.srv(ctx)
	if err != nil {
		return []lsp.DocumentSymbol{}, err
	}
	rctx, cancel := reqCtx(ctx, 10*time.Second)
	defer cancel()
	return srv.DocumentSymbols(rctx, path)
}

// WorkspaceSymbols searches symbols across the project.
func (s *LangService) WorkspaceSymbols(ctx context.Context, query string) ([]langserver.Symbol, error) {
	ls, srv, err := s.srv(ctx)
	if err != nil {
		return []langserver.Symbol{}, err
	}
	rctx, cancel := reqCtx(ctx, 10*time.Second)
	defer cancel()
	syms, err := srv.WorkspaceSymbols(rctx, query)
	for i := range syms {
		syms[i].Location.Path = ls.toEditor(syms[i].Location.Path)
	}
	return syms, err
}

// PrepareRename checks that the symbol at a position can be renamed.
func (s *LangService) PrepareRename(ctx context.Context, path string, line, char int) (lsp.PrepareRenameResult, error) {
	_, srv, err := s.srv(ctx)
	if err != nil {
		return lsp.PrepareRenameResult{}, err
	}
	rctx, cancel := reqCtx(ctx, 15*time.Second)
	defer cancel()
	return srv.PrepareRename(rctx, path, pos(line, char))
}

// Rename computes the edits that rename the symbol at a position.
func (s *LangService) Rename(ctx context.Context, path string, line, char int, newName string) ([]langserver.FileEdit, error) {
	ls, srv, err := s.srv(ctx)
	if err != nil {
		return []langserver.FileEdit{}, err
	}
	rctx, cancel := reqCtx(ctx, 60*time.Second)
	defer cancel()
	edits, err := srv.Rename(rctx, path, pos(line, char), newName)
	return ls.mapEdits(edits), err
}

// CodeActions lists quick-fixes, refactorings and source actions.
func (s *LangService) CodeActions(ctx context.Context, path string, rng lsp.Range, diags []lsp.Diagnostic) ([]lsp.CodeAction, error) {
	_, srv, err := s.srv(ctx)
	if err != nil {
		return []lsp.CodeAction{}, err
	}
	rctx, cancel := reqCtx(ctx, 15*time.Second)
	defer cancel()
	return srv.CodeActions(rctx, path, rng, diags)
}

// RunCodeAction returns a code action's edits, resolving them first if
// needed, and runs its command (whose edits arrive as lang.applyEdit).
func (s *LangService) RunCodeAction(ctx context.Context, action lsp.CodeAction) ([]langserver.FileEdit, error) {
	ls, srv, err := s.srv(ctx)
	if err != nil {
		return []langserver.FileEdit{}, err
	}
	rctx, cancel := reqCtx(ctx, 60*time.Second)
	defer cancel()
	if action.Edit == nil && action.Command == nil && action.Data != nil {
		resolved, err := srv.ResolveCodeAction(rctx, action)
		if err != nil {
			return []langserver.FileEdit{}, err
		}
		action = resolved
	}
	edits := []langserver.FileEdit{}
	if action.Edit != nil {
		edits = ls.mapEdits(langserver.FileEdits(*action.Edit))
	}
	if action.Command != nil {
		if err := srv.ExecuteCommand(rctx, *action.Command); err != nil {
			return edits, err
		}
	}
	return edits, nil
}

// ApplyFileEdits applies edits to files on disk (files not open in the
// editor), preserving their line endings and BOM.
func (s *LangService) ApplyFileEdits(edits []langserver.FileEdit) error {
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
		out, err := langserver.ApplyEdits(text, fe.Edits)
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
