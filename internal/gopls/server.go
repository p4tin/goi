package gopls

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/lsp"
)

// State is the lifecycle state reported to the host.
type State string

// Server states.
const (
	StateStarting State = "starting"
	StateIndexing State = "indexing"
	StateReady    State = "ready"
	StateError    State = "error"
	StateStopped  State = "stopped"
)

// Status is a state plus a human-readable message.
type Status struct {
	State   State  `json:"state"`
	Message string `json:"message"`
}

// ErrNotReady is returned when gopls is not running or did not become
// ready in time.
var ErrNotReady = errors.New("gopls is not ready")

// readyTimeout bounds how long a request waits for the server to start.
const readyTimeout = 30 * time.Second

// Config configures a Server.
type Config struct {
	Bin  string
	Args []string
	Root string
	// Env is the full process environment (os.Environ plus project vars).
	Env []string
	// BuildFlags and Settings become the gopls configuration.
	BuildFlags []string
	Settings   map[string]any

	Log           *slog.Logger
	OnStatus      func(Status)
	OnDiagnostics func(path string, diags []lsp.Diagnostic)
	// OnApplyEdit applies a server-initiated edit; it reports success.
	OnApplyEdit func(label string, edits []FileEdit) bool

	// Backoff returns the delay before restart attempt n (1-based).
	Backoff func(n int) time.Duration
	// MaxCrashes within CrashWindow stops supervision.
	MaxCrashes  int
	CrashWindow time.Duration
}

func defaultBackoff(n int) time.Duration {
	d := time.Second << (n - 1)
	if d > 30*time.Second || d <= 0 {
		d = 30 * time.Second
	}
	return d
}

type document struct {
	lang    string
	version int
	text    string
}

// Symbol is a workspace symbol with an OS path.
type Symbol struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	ContainerName string   `json:"containerName"`
	Location      Location `json:"location"`
}

// Server supervises one gopls process.
type Server struct {
	cfg Config
	log *slog.Logger

	mu       sync.Mutex
	conn     *lsp.Conn
	proc     *os.Process
	ready    chan struct{} // closed once the current process is initialized
	status   Status
	progress map[string]lsp.WorkDoneProgress
	crashes  []time.Time
	running  bool // supervision loop active
	closed   bool
	kick     chan struct{} // wakes the loop for an immediate restart

	docMu sync.Mutex // orders document notifications
	docs  map[string]*document
}

// New returns a stopped Server. Call Start.
func New(cfg Config) *Server {
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	if cfg.Backoff == nil {
		cfg.Backoff = defaultBackoff
	}
	if cfg.MaxCrashes == 0 {
		cfg.MaxCrashes = 5
	}
	if cfg.CrashWindow == 0 {
		cfg.CrashWindow = 3 * time.Minute
	}
	return &Server{
		cfg:      cfg,
		log:      cfg.Log.With("src", "gopls"),
		ready:    make(chan struct{}),
		status:   Status{State: StateStopped},
		progress: map[string]lsp.WorkDoneProgress{},
		docs:     map[string]*document{},
		kick:     make(chan struct{}, 1),
	}
}

// Status returns the current status.
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Server) setStatus(st Status) {
	s.mu.Lock()
	if s.closed && st.State != StateStopped {
		s.mu.Unlock()
		return
	}
	changed := s.status != st
	s.status = st
	s.mu.Unlock()
	if changed && s.cfg.OnStatus != nil {
		s.cfg.OnStatus(st)
	}
}

// Start begins supervising gopls. It returns immediately.
func (s *Server) Start() {
	s.mu.Lock()
	if s.running || s.closed {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	go s.loop()
}

// Restart kills the current process (if any) and starts a new one at once,
// resetting the crash counter.
func (s *Server) Restart() {
	s.mu.Lock()
	s.crashes = nil
	proc := s.proc
	running := s.running
	s.mu.Unlock()
	if !running {
		s.Start()
		return
	}
	select {
	case s.kick <- struct{}{}:
	default:
	}
	if proc != nil {
		_ = proc.Kill()
	}
}

// Close shuts gopls down and stops supervision.
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	conn, proc, ready := s.conn, s.proc, s.ready
	s.mu.Unlock()
	if conn != nil && isClosed(ready) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := conn.Call(ctx, "shutdown", nil, nil); err == nil {
			_ = conn.Notify(ctx, "exit", nil)
		}
		cancel()
	}
	if conn != nil {
		conn.Close()
	}
	if proc != nil {
		// gopls exits on "exit"; kill it if it lingers.
		go func() {
			time.Sleep(2 * time.Second)
			_ = proc.Kill()
		}()
	}
	s.setStatus(Status{State: StateStopped})
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (s *Server) loop() {
	attempt := 0
	for {
		s.mu.Lock()
		if s.closed {
			s.running = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		start := time.Now()
		err := s.runOnce()

		s.mu.Lock()
		if s.closed {
			s.running = false
			s.mu.Unlock()
			return
		}
		kicked := false
		select {
		case <-s.kick:
			kicked = true
		default:
		}
		if kicked {
			s.mu.Unlock()
			attempt = 0
			continue
		}
		now := time.Now()
		s.crashes = append(s.crashes, now)
		cut := 0
		for cut < len(s.crashes) && now.Sub(s.crashes[cut]) > s.cfg.CrashWindow {
			cut++
		}
		s.crashes = s.crashes[cut:]
		tooMany := len(s.crashes) >= s.cfg.MaxCrashes
		if tooMany {
			s.running = false
		}
		s.mu.Unlock()

		msg := "gopls exited"
		if err != nil {
			msg = err.Error()
		}
		s.log.Warn("gopls stopped", "err", err, "uptime", time.Since(start).Round(time.Millisecond))
		if tooMany {
			s.setStatus(Status{State: StateError, Message: msg + "; stopped after repeated crashes"})
			return
		}
		if time.Since(start) > s.cfg.CrashWindow {
			attempt = 0
		}
		attempt++
		delay := s.cfg.Backoff(attempt)
		s.setStatus(Status{State: StateError, Message: fmt.Sprintf("%s; restarting in %s", msg, delay)})
		select {
		case <-time.After(delay):
		case <-s.kick:
			attempt = 0
		}
	}
}

type stdio struct {
	io.Reader
	io.WriteCloser
	closeIn func() error
}

func (p stdio) Close() error {
	err := p.WriteCloser.Close()
	if p.closeIn != nil {
		if e := p.closeIn(); err == nil {
			err = e
		}
	}
	return err
}

// runOnce starts gopls, initializes it and blocks until it exits.
func (s *Server) runOnce() error {
	s.setStatus(Status{State: StateStarting, Message: "Starting gopls…"})
	cmd := exec.Command(s.cfg.Bin, s.cfg.Args...)
	cmd.Dir = s.cfg.Root
	cmd.Env = s.cfg.Env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start gopls: %w", err)
	}
	go s.logStderr(stderr)

	conn := lsp.NewConn(stdio{Reader: stdout, WriteCloser: stdin, closeIn: stdout.Close}, s.handle, s.log)
	ready := make(chan struct{})
	s.mu.Lock()
	s.conn, s.proc, s.ready = conn, cmd.Process, ready
	s.progress = map[string]lsp.WorkDoneProgress{}
	closed := s.closed
	s.mu.Unlock()
	if closed {
		_ = cmd.Process.Kill()
	}

	exited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		conn.Close()
		exited <- err
	}()

	if err := s.initialize(conn); err != nil {
		_ = cmd.Process.Kill()
		werr := <-exited
		s.detach(conn)
		if werr != nil {
			return fmt.Errorf("initialize: %w (%v)", err, werr)
		}
		return fmt.Errorf("initialize: %w", err)
	}

	s.docMu.Lock()
	for path, d := range s.docs {
		s.notifyOpen(conn, path, d)
	}
	close(ready)
	s.docMu.Unlock()
	s.refreshProgressStatus()

	err = <-exited
	s.detach(conn)
	return err
}

func (s *Server) detach(conn *lsp.Conn) {
	s.mu.Lock()
	if s.conn == conn {
		s.conn, s.proc = nil, nil
		s.ready = make(chan struct{})
	}
	s.mu.Unlock()
}

func (s *Server) logStderr(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		s.log.Info(sc.Text(), "stream", "stderr")
	}
	if err := sc.Err(); err != nil {
		s.log.Debug("stderr read", "err", err)
	}
}

func (s *Server) workspaceFolders() []map[string]string {
	return []map[string]string{{"uri": PathToURI(s.cfg.Root), "name": filepath.Base(s.cfg.Root)}}
}

func (s *Server) configuration() map[string]any {
	cfg := map[string]any{}
	maps.Copy(cfg, s.cfg.Settings)
	if len(s.cfg.BuildFlags) > 0 {
		cfg["buildFlags"] = s.cfg.BuildFlags
	}
	return cfg
}

func (s *Server) initialize(conn *lsp.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), readyTimeout)
	defer cancel()
	params := map[string]any{
		"processId":             os.Getpid(),
		"clientInfo":            map[string]string{"name": "Goi"},
		"rootUri":               PathToURI(s.cfg.Root),
		"workspaceFolders":      s.workspaceFolders(),
		"initializationOptions": s.configuration(),
		"capabilities":          clientCapabilities(),
	}
	if err := conn.Call(ctx, "initialize", params, nil); err != nil {
		return err
	}
	return conn.Notify(ctx, "initialized", map[string]any{})
}

func clientCapabilities() map[string]any {
	return map[string]any{
		"general": map[string]any{"positionEncodings": []string{"utf-16"}},
		"window":  map[string]any{"workDoneProgress": true, "showDocument": map[string]any{"support": false}},
		"workspace": map[string]any{
			"applyEdit":              true,
			"configuration":          true,
			"workspaceFolders":       true,
			"workspaceEdit":          map[string]any{"documentChanges": true},
			"didChangeWatchedFiles":  map[string]any{"dynamicRegistration": false},
			"didChangeConfiguration": map[string]any{"dynamicRegistration": false},
			"symbol":                 map[string]any{"symbolKind": map[string]any{"valueSet": seq(1, 26)}},
			"executeCommand":         map[string]any{},
		},
		"textDocument": map[string]any{
			"synchronization": map[string]any{"didSave": true, "dynamicRegistration": false},
			"completion": map[string]any{
				"completionItem": map[string]any{
					"snippetSupport":          true,
					"documentationFormat":     []string{"markdown", "plaintext"},
					"deprecatedSupport":       true,
					"insertReplaceSupport":    false,
					"labelDetailsSupport":     false,
					"preselectSupport":        true,
					"commitCharactersSupport": false,
					"tagSupport":              map[string]any{"valueSet": []int{1}},
				},
				"completionItemKind": map[string]any{"valueSet": seq(1, 25)},
				"contextSupport":     true,
			},
			"hover":             map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
			"signatureHelp":     map[string]any{"signatureInformation": map[string]any{"documentationFormat": []string{"markdown", "plaintext"}, "parameterInformation": map[string]any{"labelOffsetSupport": true}, "activeParameterSupport": true}},
			"definition":        map[string]any{"linkSupport": false},
			"typeDefinition":    map[string]any{"linkSupport": false},
			"implementation":    map[string]any{"linkSupport": false},
			"references":        map[string]any{},
			"documentSymbol":    map[string]any{"hierarchicalDocumentSymbolSupport": true, "symbolKind": map[string]any{"valueSet": seq(1, 26)}},
			"documentHighlight": map[string]any{},
			"codeAction": map[string]any{
				"codeActionLiteralSupport": map[string]any{"codeActionKind": map[string]any{"valueSet": []string{
					"", "quickfix", "refactor", "refactor.extract", "refactor.inline", "refactor.rewrite", "source", "source.organizeImports", "source.fixAll",
				}}},
				"isPreferredSupport": true,
				"disabledSupport":    true,
				"dataSupport":        true,
				"resolveSupport":     map[string]any{"properties": []string{"edit"}},
			},
			"rename":             map[string]any{"prepareSupport": true},
			"publishDiagnostics": map[string]any{"relatedInformation": true, "tagSupport": map[string]any{"valueSet": []int{1, 2}}, "versionSupport": true},
		},
	}
}

func seq(from, to int) []int {
	out := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

// handle answers server→client requests and notifications.
func (s *Server) handle(_ context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "workspace/configuration":
		var p lsp.ConfigurationParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		out := make([]any, len(p.Items))
		for i := range p.Items {
			out[i] = s.configuration()
		}
		return out, nil
	case "workspace/workspaceFolders":
		return s.workspaceFolders(), nil
	case "window/workDoneProgress/create", "client/registerCapability", "client/unregisterCapability":
		return nil, nil
	case "$/progress":
		var p lsp.ProgressParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		var v lsp.WorkDoneProgress
		if err := json.Unmarshal(p.Value, &v); err != nil {
			return nil, nil // not work-done progress
		}
		s.onProgress(fmt.Sprint(p.Token), v)
		return nil, nil
	case "textDocument/publishDiagnostics":
		var p lsp.PublishDiagnosticsParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if p.Diagnostics == nil {
			p.Diagnostics = []lsp.Diagnostic{}
		}
		if s.cfg.OnDiagnostics != nil {
			s.cfg.OnDiagnostics(URIToPath(p.URI), p.Diagnostics)
		}
		return nil, nil
	case "workspace/applyEdit":
		var p lsp.ApplyWorkspaceEditParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		applied := false
		if s.cfg.OnApplyEdit != nil {
			applied = s.cfg.OnApplyEdit(p.Label, FileEdits(p.Edit))
		}
		return lsp.ApplyWorkspaceEditResult{Applied: applied}, nil
	case "window/logMessage", "window/showMessage":
		var p lsp.MessageParams
		if err := json.Unmarshal(params, &p); err == nil {
			if p.Type == 1 {
				s.log.Warn(p.Message, "stream", method)
			} else {
				s.log.Debug(p.Message, "stream", method)
			}
		}
		return nil, nil
	case "window/showMessageRequest":
		return nil, nil
	case "window/showDocument":
		return map[string]bool{"success": false}, nil
	}
	return nil, &lsp.ResponseError{Code: lsp.CodeMethodNotFound, Message: "unsupported: " + method}
}

func (s *Server) onProgress(token string, v lsp.WorkDoneProgress) {
	s.mu.Lock()
	switch v.Kind {
	case "begin":
		s.progress[token] = v
	case "report":
		if cur, ok := s.progress[token]; ok {
			if v.Message != "" {
				cur.Message = v.Message
			}
			if v.Percentage != nil {
				cur.Percentage = v.Percentage
			}
			s.progress[token] = cur
		}
	case "end":
		delete(s.progress, token)
	}
	s.mu.Unlock()
	s.refreshProgressStatus()
}

func (s *Server) refreshProgressStatus() {
	s.mu.Lock()
	if !isClosed(s.ready) {
		s.mu.Unlock()
		return
	}
	var parts []string
	keys := make([]string, 0, len(s.progress))
	for k := range s.progress {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := s.progress[k]
		t := p.Title
		if p.Message != "" {
			t += ": " + p.Message
		}
		if p.Percentage != nil {
			t += fmt.Sprintf(" (%d%%)", *p.Percentage)
		}
		parts = append(parts, t)
	}
	s.mu.Unlock()
	if len(parts) > 0 {
		s.setStatus(Status{State: StateIndexing, Message: strings.Join(parts, "; ")})
	} else {
		s.setStatus(Status{State: StateReady, Message: "gopls is ready"})
	}
}

// current returns the connection when the server is ready, waiting up to
// readyTimeout (or ctx).
func (s *Server) current(ctx context.Context) (*lsp.Conn, error) {
	deadline := time.NewTimer(readyTimeout)
	defer deadline.Stop()
	for {
		s.mu.Lock()
		conn, ready, closed, running := s.conn, s.ready, s.closed, s.running
		s.mu.Unlock()
		if closed || !running {
			return nil, ErrNotReady
		}
		select {
		case <-ready:
			if conn != nil {
				return conn, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, ErrNotReady
		case <-time.After(100 * time.Millisecond):
			// re-read: the process may have been replaced
		}
	}
}

func (s *Server) call(ctx context.Context, method string, params, out any) error {
	conn, err := s.current(ctx)
	if err != nil {
		return err
	}
	return conn.Call(ctx, method, params, out)
}

// readyConn returns the connection only if the server is ready now.
func (s *Server) readyConn() *lsp.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil && isClosed(s.ready) {
		return s.conn
	}
	return nil
}

func (s *Server) notifyOpen(conn *lsp.Conn, path string, d *document) {
	err := conn.Notify(context.Background(), "textDocument/didOpen", map[string]any{
		"textDocument": lsp.TextDocumentItem{URI: PathToURI(path), LanguageID: d.lang, Version: d.version, Text: d.text},
	})
	if err != nil {
		s.log.Debug("didOpen", "path", path, "err", err)
	}
}

// LanguageID returns the LSP language id for a path ("" if gopls does not
// handle it).
func LanguageID(path string) string {
	switch base := filepath.Base(path); {
	case strings.HasSuffix(base, ".go"):
		return "go"
	case base == "go.mod":
		return "go.mod"
	case base == "go.work":
		return "go.work"
	case base == "go.sum":
		return "go.sum"
	case strings.HasSuffix(base, ".tmpl") || strings.HasSuffix(base, ".gotmpl"):
		return "gotmpl"
	}
	return ""
}

// Open tracks a document and sends didOpen when gopls is ready.
func (s *Server) Open(path string, version int, text string) {
	s.docMu.Lock()
	defer s.docMu.Unlock()
	d := &document{lang: LanguageID(path), version: version, text: text}
	if _, ok := s.docs[path]; ok {
		s.docs[path] = d
		s.sendChange(path, d)
		return
	}
	s.docs[path] = d
	if conn := s.readyConn(); conn != nil {
		s.notifyOpen(conn, path, d)
	}
}

// Change replaces a tracked document's text (full sync).
func (s *Server) Change(path string, version int, text string) {
	s.docMu.Lock()
	defer s.docMu.Unlock()
	d, ok := s.docs[path]
	if !ok {
		d = &document{lang: LanguageID(path)}
		s.docs[path] = d
		d.version, d.text = version, text
		if conn := s.readyConn(); conn != nil {
			s.notifyOpen(conn, path, d)
		}
		return
	}
	if version <= d.version && text == d.text {
		return
	}
	d.version, d.text = version, text
	s.sendChange(path, d)
}

func (s *Server) sendChange(path string, d *document) {
	conn := s.readyConn()
	if conn == nil {
		return
	}
	err := conn.Notify(context.Background(), "textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": PathToURI(path), "version": d.version},
		"contentChanges": []map[string]string{{"text": d.text}},
	})
	if err != nil {
		s.log.Debug("didChange", "path", path, "err", err)
	}
}

// Version returns the tracked version of path (-1 if untracked).
func (s *Server) Version(path string) int {
	s.docMu.Lock()
	defer s.docMu.Unlock()
	if d, ok := s.docs[path]; ok {
		return d.version
	}
	return -1
}

// Save sends didSave for a tracked document.
func (s *Server) Save(path string) {
	s.docMu.Lock()
	defer s.docMu.Unlock()
	if _, ok := s.docs[path]; !ok {
		return
	}
	if conn := s.readyConn(); conn != nil {
		_ = conn.Notify(context.Background(), "textDocument/didSave", map[string]any{"textDocument": lsp.TextDocumentIdentifier{URI: PathToURI(path)}})
	}
}

// CloseDoc stops tracking a document and sends didClose.
func (s *Server) CloseDoc(path string) {
	s.docMu.Lock()
	defer s.docMu.Unlock()
	if _, ok := s.docs[path]; !ok {
		return
	}
	delete(s.docs, path)
	if conn := s.readyConn(); conn != nil {
		_ = conn.Notify(context.Background(), "textDocument/didClose", map[string]any{"textDocument": lsp.TextDocumentIdentifier{URI: PathToURI(path)}})
	}
}

// WatchedFilesChanged forwards disk changes to gopls.
func (s *Server) WatchedFilesChanged(events []lsp.FileEvent) {
	if len(events) == 0 {
		return
	}
	if conn := s.readyConn(); conn != nil {
		_ = conn.Notify(context.Background(), "workspace/didChangeWatchedFiles", map[string]any{"changes": events})
	}
}

func posParams(path string, pos lsp.Position) lsp.TextDocumentPositionParams {
	return lsp.TextDocumentPositionParams{TextDocument: lsp.TextDocumentIdentifier{URI: PathToURI(path)}, Position: pos}
}

// Completion returns completion candidates at pos.
func (s *Server) Completion(ctx context.Context, path string, pos lsp.Position) (lsp.CompletionList, error) {
	var out lsp.CompletionList
	err := s.call(ctx, "textDocument/completion", posParams(path, pos), &out)
	if out.Items == nil {
		out.Items = []lsp.CompletionItem{}
	}
	return out, err
}

// Hover returns documentation for the symbol at pos (nil if none).
func (s *Server) Hover(ctx context.Context, path string, pos lsp.Position) (*lsp.Hover, error) {
	var out *lsp.Hover
	err := s.call(ctx, "textDocument/hover", posParams(path, pos), &out)
	return out, err
}

// SignatureHelp returns the signature of the call at pos (nil if none).
func (s *Server) SignatureHelp(ctx context.Context, path string, pos lsp.Position) (*lsp.SignatureHelp, error) {
	var out *lsp.SignatureHelp
	err := s.call(ctx, "textDocument/signatureHelp", posParams(path, pos), &out)
	return out, err
}

func (s *Server) locations(ctx context.Context, method string, params any) ([]Location, error) {
	var raw lsp.Locations
	if err := s.call(ctx, method, params, &raw); err != nil {
		return []Location{}, err
	}
	out := make([]Location, 0, len(raw))
	for _, l := range raw {
		out = append(out, Location{Path: URIToPath(l.URI), Range: l.Range})
	}
	return out, nil
}

// Definition returns the declaration(s) of the symbol at pos.
func (s *Server) Definition(ctx context.Context, path string, pos lsp.Position) ([]Location, error) {
	return s.locations(ctx, "textDocument/definition", posParams(path, pos))
}

// TypeDefinition returns the declaration of the type of the symbol at pos.
func (s *Server) TypeDefinition(ctx context.Context, path string, pos lsp.Position) ([]Location, error) {
	return s.locations(ctx, "textDocument/typeDefinition", posParams(path, pos))
}

// Implementation returns implementations (or implemented interfaces).
func (s *Server) Implementation(ctx context.Context, path string, pos lsp.Position) ([]Location, error) {
	return s.locations(ctx, "textDocument/implementation", posParams(path, pos))
}

// References returns the usages of the symbol at pos, excluding its
// declaration.
func (s *Server) References(ctx context.Context, path string, pos lsp.Position) ([]Location, error) {
	p := map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: PathToURI(path)},
		"position":     pos,
		"context":      map[string]bool{"includeDeclaration": false},
	}
	return s.locations(ctx, "textDocument/references", p)
}

// DocumentHighlight returns occurrences of the symbol at pos in its file.
func (s *Server) DocumentHighlight(ctx context.Context, path string, pos lsp.Position) ([]lsp.DocumentHighlight, error) {
	out := []lsp.DocumentHighlight{}
	err := s.call(ctx, "textDocument/documentHighlight", posParams(path, pos), &out)
	if out == nil {
		out = []lsp.DocumentHighlight{}
	}
	return out, err
}

// DocumentSymbols returns the symbol tree of path.
func (s *Server) DocumentSymbols(ctx context.Context, path string) ([]lsp.DocumentSymbol, error) {
	out := []lsp.DocumentSymbol{}
	err := s.call(ctx, "textDocument/documentSymbol", map[string]any{"textDocument": lsp.TextDocumentIdentifier{URI: PathToURI(path)}}, &out)
	if out == nil {
		out = []lsp.DocumentSymbol{}
	}
	return out, err
}

// WorkspaceSymbols searches symbols across the workspace.
func (s *Server) WorkspaceSymbols(ctx context.Context, query string) ([]Symbol, error) {
	var raw []lsp.SymbolInformation
	if err := s.call(ctx, "workspace/symbol", map[string]string{"query": query}, &raw); err != nil {
		return []Symbol{}, err
	}
	out := make([]Symbol, 0, len(raw))
	for _, r := range raw {
		out = append(out, Symbol{Name: r.Name, Kind: r.Kind, ContainerName: r.ContainerName, Location: Location{Path: URIToPath(r.Location.URI), Range: r.Location.Range}})
	}
	return out, nil
}

// PrepareRename checks whether the symbol at pos can be renamed.
func (s *Server) PrepareRename(ctx context.Context, path string, pos lsp.Position) (lsp.PrepareRenameResult, error) {
	var out lsp.PrepareRenameResult
	err := s.call(ctx, "textDocument/prepareRename", posParams(path, pos), &out)
	return out, err
}

// Rename computes the edits renaming the symbol at pos.
func (s *Server) Rename(ctx context.Context, path string, pos lsp.Position, newName string) ([]FileEdit, error) {
	var we lsp.WorkspaceEdit
	p := map[string]any{"textDocument": lsp.TextDocumentIdentifier{URI: PathToURI(path)}, "position": pos, "newName": newName}
	if err := s.call(ctx, "textDocument/rename", p, &we); err != nil {
		return []FileEdit{}, err
	}
	return FileEdits(we), nil
}

// CodeActions lists code actions for rng with the given diagnostics.
func (s *Server) CodeActions(ctx context.Context, path string, rng lsp.Range, diags []lsp.Diagnostic) ([]lsp.CodeAction, error) {
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	var out lsp.CodeActions
	p := lsp.CodeActionParams{TextDocument: lsp.TextDocumentIdentifier{URI: PathToURI(path)}, Range: rng, Context: lsp.CodeActionContext{Diagnostics: diags, TriggerKind: 1}}
	err := s.call(ctx, "textDocument/codeAction", p, &out)
	if out == nil {
		out = lsp.CodeActions{}
	}
	return out, err
}

// ResolveCodeAction fills in a code action's edit.
func (s *Server) ResolveCodeAction(ctx context.Context, a lsp.CodeAction) (lsp.CodeAction, error) {
	var out lsp.CodeAction
	err := s.call(ctx, "codeAction/resolve", a, &out)
	return out, err
}

// ExecuteCommand runs a server command. Edits it produces arrive through
// OnApplyEdit.
func (s *Server) ExecuteCommand(ctx context.Context, cmd lsp.Command) error {
	args := cmd.Arguments
	if args == nil {
		args = []any{}
	}
	return s.call(ctx, "workspace/executeCommand", map[string]any{"command": cmd.Command, "arguments": args}, nil)
}
