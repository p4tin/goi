// Package debugger drives Delve over DAP: it starts `dlv dap` (or connects
// to a headless Delve for remote attach), performs the launch/attach and
// configuration handshake, and exposes the requests the UI needs, including
// run-to-cursor built from a temporary breakpoint. It reports adapter
// events and dlv's own output through callbacks.
package debugger

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/dap"
)

// Event is an adapter event forwarded to the owner.
type Event = dap.Event

// Config describes a session.
type Config struct {
	// Dlv is the Delve binary (unused for remote attach).
	Dlv string
	// Env and Dir apply to the dlv process.
	Env []string
	Dir string
	// Addr is host:port of a headless Delve for remote attach; when set no
	// dlv process is started.
	Addr string
	Log  *slog.Logger
	// OnEvent receives adapter events in order.
	OnEvent func(Event)
	// OnOutput receives dlv's own stdout/stderr lines (not program output,
	// which arrives as "output" events).
	OnOutput func(stream, text string)
	// StartTimeout bounds waiting for dlv to listen (default 30 s).
	StartTimeout time.Duration
}

// Session is a debug session.
type Session struct {
	cfg  Config
	conn *dap.Conn
	proc *exec.Cmd
	log  *slog.Logger

	caps        dap.Capabilities
	initialized chan struct{}
	initOnce    sync.Once

	mu sync.Mutex
	// launchLog collects adapter output during launch so launch errors can
	// include the build output Delve only sends to the console.
	launchLog *strings.Builder
	lineBPs   map[string][]dap.SourceBreakpoint
	// links maps symlink-resolved directories back to the paths the editor
	// uses, so frames and breakpoints report the editor's paths.
	links    []link
	temp     *tempBreakpoint
	procDone chan struct{}
	closed   bool
}

type link struct{ real, orig string }

// toAdapter returns p with symlinks resolved, remembering the mapping of
// its directory for fromAdapter.
func (s *Session) toAdapter(p string) string {
	if !filepath.IsAbs(p) {
		return p
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil || real == p {
		return p
	}
	rd, od := filepath.Dir(real), filepath.Dir(p)
	if filepath.Base(real) != filepath.Base(p) {
		return real // the file itself is a symlink; nothing to learn
	}
	s.mu.Lock()
	known := false
	for _, l := range s.links {
		if l.real == rd {
			known = true
		}
	}
	if !known {
		s.links = append(s.links, link{real: rd, orig: od})
	}
	s.mu.Unlock()
	return real
}

// fromAdapter maps an adapter path back to the editor's path.
func (s *Session) fromAdapter(p string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	best := link{}
	for _, l := range s.links {
		if (p == l.real || strings.HasPrefix(p, l.real+string(filepath.Separator))) && len(l.real) > len(best.real) {
			best = l
		}
	}
	if best.real == "" {
		return p
	}
	return best.orig + p[len(best.real):]
}

func (s *Session) mapSource(src *dap.Source) {
	if src != nil && src.Path != "" {
		src.Path = s.fromAdapter(src.Path)
	}
}

type tempBreakpoint struct {
	path string
}

var listenRE = regexp.MustCompile(`listening at:\s*(\S+:\d+)`)

// Start starts dlv dap (or connects to cfg.Addr) and initializes the
// adapter.
func Start(ctx context.Context, cfg Config) (*Session, error) {
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	if cfg.StartTimeout <= 0 {
		cfg.StartTimeout = 30 * time.Second
	}
	s := &Session{cfg: cfg, log: cfg.Log, initialized: make(chan struct{}), lineBPs: map[string][]dap.SourceBreakpoint{}, procDone: make(chan struct{})}
	addr := cfg.Addr
	if addr == "" {
		var err error
		addr, err = s.startDlv(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		close(s.procDone)
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		s.kill()
		return nil, fmt.Errorf("debugger: connect to %s: %w", addr, err)
	}
	s.conn = dap.NewConn(nc, s.onEvent, s.onReverse, s.log)
	ictx, cancel := context.WithTimeout(ctx, cfg.StartTimeout)
	defer cancel()
	err = s.conn.Call(ictx, "initialize", dap.InitializeArguments{
		ClientID: "goi", ClientName: "Goi", AdapterID: "go", Locale: "en",
		LinesStartAt1: true, ColumnsStartAt1: true, PathFormat: "path",
		SupportsVariableType: true,
	}, &s.caps)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("debugger: initialize: %w", err)
	}
	return s, nil
}

func (s *Session) startDlv(ctx context.Context) (string, error) {
	cmd := exec.Command(s.cfg.Dlv, "dap", "--listen=127.0.0.1:0")
	cmd.Dir = s.cfg.Dir
	cmd.Env = s.cfg.Env
	setProcAttr(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("debugger: start %s: %w", s.cfg.Dlv, err)
	}
	s.proc = cmd
	addrc := make(chan string, 1)
	var early strings.Builder
	var earlyMu sync.Mutex
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		found := false
		for sc.Scan() {
			line := sc.Text()
			if !found {
				if m := listenRE.FindStringSubmatch(line); m != nil {
					found = true
					addrc <- m[1]
					continue
				}
				earlyMu.Lock()
				early.WriteString(line + "\n")
				earlyMu.Unlock()
			}
			s.output("stdout", line+"\n")
		}
	}()
	go s.pump("stderr", stderr, &early, &earlyMu)
	go func() {
		_ = cmd.Wait()
		close(s.procDone)
	}()
	select {
	case addr := <-addrc:
		return addr, nil
	case <-s.procDone:
		earlyMu.Lock()
		defer earlyMu.Unlock()
		return "", fmt.Errorf("debugger: dlv exited before listening: %s", strings.TrimSpace(early.String()))
	case <-time.After(s.cfg.StartTimeout):
		s.kill()
		return "", errors.New("debugger: timed out waiting for dlv to listen")
	case <-ctx.Done():
		s.kill()
		return "", ctx.Err()
	}
}

func (s *Session) pump(stream string, r io.Reader, early *strings.Builder, mu *sync.Mutex) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		mu.Lock()
		if early.Len() < 8192 {
			early.WriteString(sc.Text() + "\n")
		}
		mu.Unlock()
		s.output(stream, sc.Text()+"\n")
	}
}

func (s *Session) output(stream, text string) {
	if s.cfg.OnOutput != nil {
		s.cfg.OnOutput(stream, text)
	}
}

func (s *Session) onReverse(command string, _ json.RawMessage) (any, error) {
	return nil, fmt.Errorf("%s is not supported by Goi", command)
}

func (s *Session) onEvent(ev Event) {
	switch ev.Event {
	case "output":
		s.mu.Lock()
		if s.launchLog != nil && s.launchLog.Len() < 16<<10 {
			var o dap.OutputEvent
			if json.Unmarshal(ev.Body, &o) == nil && o.Category != "stdout" {
				s.launchLog.WriteString(o.Output)
			}
		}
		s.mu.Unlock()
	case "initialized":
		s.initOnce.Do(func() { close(s.initialized) })
	case "stopped", "terminated", "exited":
		s.mu.Lock()
		t := s.temp
		s.temp = nil
		s.mu.Unlock()
		if t != nil && ev.Event == "stopped" {
			s.restoreBreakpoints(t.path)
		}
	}
	if s.cfg.OnEvent != nil {
		s.cfg.OnEvent(ev)
	}
}

// Capabilities are the adapter's capabilities.
func (s *Session) Capabilities() dap.Capabilities { return s.caps }

// Done is closed when the connection to the adapter ends.
func (s *Session) Done() <-chan struct{} { return s.conn.Done() }

// Launch sends the launch request (args are dlv launch arguments) and waits
// for the initialized event.
func (s *Session) Launch(ctx context.Context, args map[string]any) error {
	clean := map[string]any{}
	for k, v := range args {
		if k != "request" {
			clean[k] = v
		}
	}
	// The go command rejects symlinked package paths as "outside main
	// module" (macOS /var → /private/var), so pass real paths.
	for _, k := range []string{"program", "cwd"} {
		if p, ok := clean[k].(string); ok && filepath.IsAbs(p) {
			if real, err := filepath.EvalSymlinks(p); err == nil {
				clean[k] = real
				if real != p {
					s.mu.Lock()
					s.links = append(s.links, link{real: real, orig: p})
					s.mu.Unlock()
				}
			}
		}
	}
	s.mu.Lock()
	s.launchLog = &strings.Builder{}
	s.mu.Unlock()
	err := s.conn.Call(ctx, "launch", clean, nil)
	if err != nil {
		s.conn.DrainEvents(500 * time.Millisecond)
	}
	s.mu.Lock()
	detail := strings.TrimSpace(s.launchLog.String())
	s.launchLog = nil
	s.mu.Unlock()
	if err != nil {
		if detail != "" {
			return fmt.Errorf("%w\n%s", err, detail)
		}
		return err
	}
	return s.waitInitialized(ctx)
}

// Attach attaches to the remote headless Delve given by Config.Addr.
func (s *Session) Attach(ctx context.Context) error {
	if err := s.conn.Call(ctx, "attach", map[string]any{"mode": "remote"}, nil); err != nil {
		return err
	}
	return s.waitInitialized(ctx)
}

func (s *Session) waitInitialized(ctx context.Context) error {
	select {
	case <-s.initialized:
		return nil
	case <-s.conn.Done():
		return errors.New("debugger: adapter closed during launch")
	case <-time.After(s.cfg.StartTimeout):
		return errors.New("debugger: timed out waiting for the adapter to initialize")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConfigurationDone ends the configuration phase; the program starts.
func (s *Session) ConfigurationDone(ctx context.Context) error {
	return s.conn.Call(ctx, "configurationDone", nil, nil)
}

// SetBreakpoints replaces the line breakpoints of path.
func (s *Session) SetBreakpoints(ctx context.Context, path string, bps []dap.SourceBreakpoint) ([]dap.Breakpoint, error) {
	if bps == nil {
		bps = []dap.SourceBreakpoint{}
	}
	s.mu.Lock()
	s.lineBPs[path] = bps
	s.mu.Unlock()
	return s.setBreakpoints(ctx, path, bps)
}

func (s *Session) setBreakpoints(ctx context.Context, path string, bps []dap.SourceBreakpoint) ([]dap.Breakpoint, error) {
	var out dap.BreakpointsResponse
	err := s.conn.Call(ctx, "setBreakpoints", dap.SetBreakpointsArguments{Source: dap.Source{Path: s.toAdapter(path)}, Breakpoints: bps}, &out)
	return s.mapBPs(out.Breakpoints), err
}

func (s *Session) mapBPs(b []dap.Breakpoint) []dap.Breakpoint {
	b = nonNilBPs(b)
	for i := range b {
		s.mapSource(b[i].Source)
	}
	return b
}

func (s *Session) restoreBreakpoints(path string) {
	s.mu.Lock()
	bps := s.lineBPs[path]
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.setBreakpoints(ctx, path, bps); err != nil {
		s.log.Debug("debugger: restore breakpoints", "path", path, "err", err)
	}
}

// SetFunctionBreakpoints replaces all function breakpoints.
func (s *Session) SetFunctionBreakpoints(ctx context.Context, bps []dap.FunctionBreakpoint) ([]dap.Breakpoint, error) {
	if bps == nil {
		bps = []dap.FunctionBreakpoint{}
	}
	var out dap.BreakpointsResponse
	err := s.conn.Call(ctx, "setFunctionBreakpoints", map[string]any{"breakpoints": bps}, &out)
	return s.mapBPs(out.Breakpoints), err
}

// SetExceptionBreakpoints enables exception filters.
func (s *Session) SetExceptionBreakpoints(ctx context.Context, filters []string) error {
	if filters == nil {
		filters = []string{}
	}
	return s.conn.Call(ctx, "setExceptionBreakpoints", map[string]any{"filters": filters}, nil)
}

func nonNilBPs(b []dap.Breakpoint) []dap.Breakpoint {
	if b == nil {
		return []dap.Breakpoint{}
	}
	return b
}

// Threads lists threads (goroutines).
func (s *Session) Threads(ctx context.Context) ([]dap.Thread, error) {
	var out struct {
		Threads []dap.Thread `json:"threads"`
	}
	err := s.conn.Call(ctx, "threads", nil, &out)
	if out.Threads == nil {
		out.Threads = []dap.Thread{}
	}
	return out.Threads, err
}

// StackTrace returns frames of a thread (levels 0 = all).
func (s *Session) StackTrace(ctx context.Context, threadID, start, levels int) (dap.StackTraceResponse, error) {
	var out dap.StackTraceResponse
	err := s.conn.Call(ctx, "stackTrace", map[string]any{"threadId": threadID, "startFrame": start, "levels": levels}, &out)
	if out.StackFrames == nil {
		out.StackFrames = []dap.StackFrame{}
	}
	for i := range out.StackFrames {
		s.mapSource(out.StackFrames[i].Source)
	}
	return out, err
}

// Scopes returns the scopes of a frame.
func (s *Session) Scopes(ctx context.Context, frameID int) ([]dap.Scope, error) {
	var out struct {
		Scopes []dap.Scope `json:"scopes"`
	}
	err := s.conn.Call(ctx, "scopes", map[string]any{"frameId": frameID}, &out)
	if out.Scopes == nil {
		out.Scopes = []dap.Scope{}
	}
	return out.Scopes, err
}

// Variables returns the children of a variables reference.
func (s *Session) Variables(ctx context.Context, ref int) ([]dap.Variable, error) {
	var out struct {
		Variables []dap.Variable `json:"variables"`
	}
	err := s.conn.Call(ctx, "variables", map[string]any{"variablesReference": ref}, &out)
	if out.Variables == nil {
		out.Variables = []dap.Variable{}
	}
	return out.Variables, err
}

// Evaluate evaluates expr in a frame. evalContext is "watch", "repl",
// "hover" or "clipboard".
func (s *Session) Evaluate(ctx context.Context, expr string, frameID int, evalContext string) (dap.EvaluateResponse, error) {
	var out dap.EvaluateResponse
	args := map[string]any{"expression": expr, "context": evalContext}
	if frameID > 0 {
		args["frameId"] = frameID
	}
	err := s.conn.Call(ctx, "evaluate", args, &out)
	return out, err
}

// SetVariable sets a variable in the container ref.
func (s *Session) SetVariable(ctx context.Context, ref int, name, value string) (dap.SetVariableResponse, error) {
	var out dap.SetVariableResponse
	err := s.conn.Call(ctx, "setVariable", map[string]any{"variablesReference": ref, "name": name, "value": value}, &out)
	return out, err
}

// ExceptionInfo describes why threadID stopped on an exception (panic).
func (s *Session) ExceptionInfo(ctx context.Context, threadID int) (dap.ExceptionInfoResponse, error) {
	var out dap.ExceptionInfoResponse
	err := s.conn.Call(ctx, "exceptionInfo", map[string]any{"threadId": threadID}, &out)
	return out, err
}

// Continue resumes all goroutines.
func (s *Session) Continue(ctx context.Context, threadID int) error {
	return s.conn.Call(ctx, "continue", map[string]any{"threadId": threadID}, nil)
}

// Next steps over.
func (s *Session) Next(ctx context.Context, threadID int) error {
	return s.conn.Call(ctx, "next", map[string]any{"threadId": threadID}, nil)
}

// StepIn steps into.
func (s *Session) StepIn(ctx context.Context, threadID int) error {
	return s.conn.Call(ctx, "stepIn", map[string]any{"threadId": threadID}, nil)
}

// StepOut steps out.
func (s *Session) StepOut(ctx context.Context, threadID int) error {
	return s.conn.Call(ctx, "stepOut", map[string]any{"threadId": threadID}, nil)
}

// Pause pauses the program.
func (s *Session) Pause(ctx context.Context, threadID int) error {
	return s.conn.Call(ctx, "pause", map[string]any{"threadId": threadID}, nil)
}

// RunToCursor continues to path:line using a temporary breakpoint added to
// the file's breakpoints; the file's breakpoints are restored at the next
// stop.
func (s *Session) RunToCursor(ctx context.Context, threadID int, path string, line int) error {
	s.mu.Lock()
	bps := append([]dap.SourceBreakpoint{}, s.lineBPs[path]...)
	s.mu.Unlock()
	has := false
	for _, b := range bps {
		if b.Line == line && b.Condition == "" && b.HitCondition == "" && b.LogMessage == "" {
			has = true
		}
	}
	if !has {
		bps = append(bps, dap.SourceBreakpoint{Line: line})
	}
	res, err := s.setBreakpoints(ctx, path, bps)
	if err != nil {
		return err
	}
	if !has {
		if r := res[len(res)-1]; len(res) == len(bps) && !r.Verified {
			s.restoreBreakpoints(path)
			msg := r.Message
			if msg == "" {
				msg = "no code at this line"
			}
			return fmt.Errorf("cannot run to line %d: %s", line, msg)
		}
		s.mu.Lock()
		s.temp = &tempBreakpoint{path: path}
		s.mu.Unlock()
	}
	return s.Continue(ctx, threadID)
}

// Disconnect ends the session. terminate stops the debuggee (launch); for
// remote attach false detaches and leaves the program running.
func (s *Session) Disconnect(ctx context.Context, terminate bool) error {
	err := s.conn.Call(ctx, "disconnect", map[string]any{"terminateDebuggee": terminate}, nil)
	s.Close()
	return err
}

// Close closes the connection and stops dlv (after a short grace period).
func (s *Session) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
	}
	select {
	case <-s.procDone:
	case <-time.After(3 * time.Second):
		s.kill()
		<-s.procDone
	}
}

func (s *Session) kill() {
	if s.proc != nil && s.proc.Process != nil {
		killTree(s.proc)
	}
}
