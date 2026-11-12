package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/dap"
	"github.com/p4tin/goi/internal/debugger"
	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/process"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/internal/toolbin"
)

// DlvModule is what "Install Delve" installs.
const DlvModule = "github.com/go-delve/delve/cmd/dlv@latest"

// Debug states (debug.state).
const (
	DebugStarting = "starting"
	DebugRunning  = "running"
	DebugStopped  = "stopped"
	DebugExited   = "exited"
)

// panicFunction is the function breakpoint behind "Break on panic".
const panicFunction = "runtime.gopanic"

// LineBreakpoint is a line breakpoint as the UI keeps it.
type LineBreakpoint struct {
	Line         int    `json:"line"`
	Condition    string `json:"condition"`
	HitCondition string `json:"hitCondition"`
	LogMessage   string `json:"logMessage"`
	Enabled      bool   `json:"enabled"`
}

// FunctionBreakpoint is a function breakpoint as the UI keeps it.
type FunctionBreakpoint struct {
	Name      string `json:"name"`
	Condition string `json:"condition"`
	Enabled   bool   `json:"enabled"`
}

// DebugCaps are the adapter capabilities the UI needs.
type DebugCaps struct {
	SupportsSetVariable bool `json:"supportsSetVariable"`
	SupportsGotoTargets bool `json:"supportsGotoTargets"`
	SupportsRestart     bool `json:"supportsRestart"`
}

// DebugStateEvent is the payload of debug.state.
type DebugStateEvent struct {
	ID          string    `json:"id"`
	State       string    `json:"state"`
	ThreadID    int       `json:"threadId"`
	Reason      string    `json:"reason"`
	Description string    `json:"description"`
	Text        string    `json:"text"`
	ExitCode    *int      `json:"exitCode"`
	Remote      bool      `json:"remote"`
	Caps        DebugCaps `json:"caps"`
}

// DebugBreakpointsEvent is the payload of debug.breakpoints: the adapter's
// verification of a file's (or the function) breakpoints, in request order.
type DebugBreakpointsEvent struct {
	ID          string           `json:"id"`
	Path        string           `json:"path"`
	Function    bool             `json:"function"`
	Breakpoints []dap.Breakpoint `json:"breakpoints"`
}

// DlvStatus describes the Delve installation.
type DlvStatus struct {
	Path       string `json:"path"`
	Found      bool   `json:"found"`
	Installing bool   `json:"installing"`
	Message    string `json:"message"`
}

// debugManager owns the project's breakpoints and debug sessions.
type debugManager struct {
	app *App
	win *window

	mu         sync.Mutex
	lines      map[string][]LineBreakpoint
	funcs      []FunctionBreakpoint
	panicBreak bool
	sessions   map[string]*debugSession
	installing bool
}

type debugSession struct {
	m      *debugManager
	id     string
	run    *runInstance
	remote bool

	mu       sync.Mutex
	sess     *debugger.Session
	state    string
	threadID int
	exitCode *int
	stopping bool
}

func newDebugManager(w *window) *debugManager {
	return &debugManager{app: w.app, win: w, lines: map[string][]LineBreakpoint{}, sessions: map[string]*debugSession{}}
}

func (m *debugManager) close() {
	m.mu.Lock()
	list := make([]*debugSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.Unlock()
	for _, s := range list {
		s.kill()
	}
}

// resolveDlv finds Delve with the window's debug.dlvPath setting and SDK.
func (w *window) resolveDlv() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	opts := toolbin.Options{Name: "dlv", Setting: w.settings.Effective().Debug.DlvPath, ToolsDir: w.app.toolsDir()}
	if r, err := w.runner(ctx); err == nil {
		if ge, err := r.GoEnv(ctx); err == nil {
			opts.GOBIN, opts.GOPATH = ge["GOBIN"], ge["GOPATH"]
		}
	}
	return toolbin.Resolve(opts)
}

func (s *debugSession) emit(ev DebugStateEvent) {
	ev.ID = s.id
	ev.Remote = s.remote
	s.mu.Lock()
	if ev.State != "" {
		s.state = ev.State
	}
	if ev.ExitCode == nil {
		ev.ExitCode = s.exitCode
	}
	if s.sess != nil {
		c := s.sess.Capabilities()
		ev.Caps = DebugCaps{SupportsSetVariable: c.SupportsSetVariable, SupportsGotoTargets: c.SupportsGotoTargetsRequest, SupportsRestart: true}
	}
	s.mu.Unlock()
	s.m.win.bus.Emit(events.TopicDebugState, ev)
}

func (s *debugSession) session() (*debugger.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sess == nil {
		return nil, errors.New("the debugger is not ready")
	}
	return s.sess, nil
}

func (s *debugSession) onEvent(ev debugger.Event) {
	switch ev.Event {
	case "output":
		var o dap.OutputEvent
		if json.Unmarshal(ev.Body, &o) != nil {
			return
		}
		stream := o.Category
		switch stream {
		case "stdout", "stderr":
		default:
			stream = "console"
		}
		s.run.mu.Lock()
		out := s.run.out
		s.run.mu.Unlock()
		out.Write(stream, []byte(o.Output))
	case "stopped":
		var st dap.StoppedEvent
		json.Unmarshal(ev.Body, &st)
		s.mu.Lock()
		s.threadID = st.ThreadID
		s.mu.Unlock()
		s.emit(DebugStateEvent{State: DebugStopped, ThreadID: st.ThreadID, Reason: st.Reason, Description: st.Description, Text: st.Text})
	case "continued":
		s.emit(DebugStateEvent{State: DebugRunning})
	case "exited":
		var ex dap.ExitedEvent
		json.Unmarshal(ev.Body, &ex)
		code := ex.ExitCode
		s.mu.Lock()
		s.exitCode = &code
		s.mu.Unlock()
	case "terminated":
		// Delve keeps the connection open until the client disconnects.
		s.mu.Lock()
		sess := s.sess
		s.mu.Unlock()
		if sess != nil {
			go func() {
				ctx, cancel := debugCtx()
				defer cancel()
				_ = sess.Disconnect(ctx, !s.remote)
			}()
		}
	case "breakpoint":
		var b dap.BreakpointEvent
		if json.Unmarshal(ev.Body, &b) == nil && b.Breakpoint.Source != nil {
			s.m.win.bus.Emit(events.TopicDebugBreakpoints, DebugBreakpointsEvent{ID: s.id, Path: b.Breakpoint.Source.Path, Breakpoints: []dap.Breakpoint{b.Breakpoint}})
		}
	}
}

// stop ends the session: disconnect first (terminating a launched program,
// detaching from a remote one), kill on a second call.
func (s *debugSession) stop() {
	s.mu.Lock()
	again := s.stopping
	s.stopping = true
	sess := s.sess
	s.mu.Unlock()
	if sess == nil {
		return
	}
	if again {
		go sess.Close()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sess.Disconnect(ctx, !s.remote)
	}()
}

func (s *debugSession) kill() {
	s.mu.Lock()
	s.stopping = true
	sess := s.sess
	s.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
}

// sourceBPs converts the enabled UI breakpoints of a file.
func sourceBPs(bps []LineBreakpoint) []dap.SourceBreakpoint {
	out := []dap.SourceBreakpoint{}
	for _, b := range bps {
		if !b.Enabled || b.Line < 1 {
			continue
		}
		out = append(out, dap.SourceBreakpoint{Line: b.Line, Condition: b.Condition, HitCondition: b.HitCondition, LogMessage: b.LogMessage})
	}
	return out
}

func (m *debugManager) functionBPs() []dap.FunctionBreakpoint {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []dap.FunctionBreakpoint{}
	for _, f := range m.funcs {
		if f.Enabled && f.Name != "" {
			out = append(out, dap.FunctionBreakpoint{Name: f.Name, Condition: f.Condition})
		}
	}
	if m.panicBreak {
		out = append(out, dap.FunctionBreakpoint{Name: panicFunction})
	}
	return out
}

// applyFile sends one file's breakpoints to a session and reports results.
func (m *debugManager) applyFile(ctx context.Context, s *debugSession, path string, bps []LineBreakpoint) {
	sess, err := s.session()
	if err != nil {
		return
	}
	res, err := sess.SetBreakpoints(ctx, path, sourceBPs(bps))
	if err != nil {
		m.app.log.Debug("set breakpoints", "path", path, "err", err)
		return
	}
	m.win.bus.Emit(events.TopicDebugBreakpoints, DebugBreakpointsEvent{ID: s.id, Path: path, Breakpoints: res})
}

func (m *debugManager) applyFunctions(ctx context.Context, s *debugSession) {
	sess, err := s.session()
	if err != nil {
		return
	}
	res, err := sess.SetFunctionBreakpoints(ctx, m.functionBPs())
	if err != nil {
		m.app.log.Debug("set function breakpoints", "err", err)
		return
	}
	m.win.bus.Emit(events.TopicDebugBreakpoints, DebugBreakpointsEvent{ID: s.id, Function: true, Breakpoints: res})
}

func (m *debugManager) active() []*debugSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*debugSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

// startDebug runs a debug session for a run whose before-launch tasks are
// done. It returns when the session ends, finishing the run.
func (a *App) startDebug(r *runInstance, pctx runconfig.Context) {
	sess, err := a.current()
	if err != nil || sess.debug == nil {
		r.finish(RunFailed, "no project", nil)
		return
	}
	m := sess.debug
	r.mu.Lock()
	cfg := r.cfg
	r.mu.Unlock()
	remote := cfg.Type == runconfig.TypeRemote
	ds := &debugSession{m: m, id: r.id, run: r, remote: remote, state: DebugStarting}
	m.mu.Lock()
	m.sessions[ds.id] = ds
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		if m.sessions[ds.id] == ds {
			delete(m.sessions, ds.id)
		}
		m.mu.Unlock()
	}()
	fail := func(msg string) {
		r.system(msg + "\n")
		ds.emit(DebugStateEvent{State: DebugExited})
		r.finish(RunFailed, msg, nil)
	}

	dc := debugger.Config{Env: pctx.Env, Dir: r.m.root, Log: a.log.With("src", "dlv"), OnEvent: ds.onEvent,
		OnOutput: func(stream, text string) {
			r.mu.Lock()
			out := r.out
			r.mu.Unlock()
			out.Write("console", []byte(text))
		}}
	var launch map[string]any
	if remote {
		dc.Addr = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
		r.system("Connecting to " + dc.Addr + "\n")
	} else {
		dlv, err := r.m.win.resolveDlv()
		if err != nil {
			fail(err.Error())
			return
		}
		dc.Dlv = dlv
		if err := r.m.ensureTempDir(); err != nil {
			fail(err.Error())
			return
		}
		bin := filepath.Join(r.m.tempDir, "debug_"+runconfig.Slug(cfg.Name))
		if launch, err = cfg.DebugLaunch(pctx, bin); err != nil {
			fail(err.Error())
			return
		}
		defer os.Remove(bin)
		r.setState(RunBuilding, "")
	}
	r.mu.Lock()
	r.stopHook = ds.stop
	stopped := r.stopped
	r.mu.Unlock()
	if stopped {
		r.finish(RunExited, "Stopped", nil)
		return
	}

	ctx := context.Background()
	s, err := debugger.Start(ctx, dc)
	if err != nil {
		fail(err.Error())
		return
	}
	ds.mu.Lock()
	ds.sess = s
	ds.mu.Unlock()
	ds.emit(DebugStateEvent{State: DebugStarting})
	lctx, cancel := context.WithTimeout(ctx, 10*time.Minute) // builds can be slow
	if remote {
		err = s.Attach(lctx)
	} else {
		err = s.Launch(lctx, launch)
	}
	cancel()
	if err != nil {
		s.Close()
		fail(err.Error())
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	m.mu.Lock()
	files := make(map[string][]LineBreakpoint, len(m.lines))
	for p, b := range m.lines {
		files[p] = b
	}
	m.mu.Unlock()
	for p, b := range files {
		m.applyFile(cctx, ds, p, b)
	}
	m.applyFunctions(cctx, ds)
	err = s.ConfigurationDone(cctx)
	cancel()
	if err != nil {
		s.Close()
		fail(err.Error())
		return
	}
	r.mu.Lock()
	r.command = "dlv dap"
	r.mu.Unlock()
	r.setState(RunRunning, "")
	ds.emit(DebugStateEvent{State: DebugRunning})

	<-s.Done()
	s.Close()
	ds.mu.Lock()
	code := ds.exitCode
	ds.mu.Unlock()
	ds.emit(DebugStateEvent{State: DebugExited})
	var exit *process.ExitInfo
	if code != nil {
		exit = &process.ExitInfo{Code: *code}
	}
	msg := ""
	if remote {
		msg = "Disconnected"
	}
	r.finish(RunExited, msg, exit)
}

// DebugService exposes debugging for the open project.
type DebugService struct {
	app *App
}

func (s *DebugService) manager() (*debugManager, error) {
	sess, err := s.app.current()
	if err != nil {
		return nil, err
	}
	if sess.debug == nil {
		return nil, errNoProject
	}
	return sess.debug, nil
}

func (s *DebugService) get(id string) (*debugSession, *debugger.Session, error) {
	m, err := s.manager()
	if err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	ds := m.sessions[id]
	m.mu.Unlock()
	if ds == nil {
		return nil, nil, fmt.Errorf("no debug session %s", id)
	}
	sess, err := ds.session()
	return ds, sess, err
}

func debugCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// DlvStatus reports whether Delve is available.
func (s *DebugService) DlvStatus() DlvStatus {
	m, _ := s.manager()
	installing := false
	if m != nil {
		m.mu.Lock()
		installing = m.installing
		m.mu.Unlock()
	}
	p, err := s.app.mainWindow().resolveDlv()
	if err != nil {
		return DlvStatus{Installing: installing, Message: err.Error()}
	}
	return DlvStatus{Path: p, Found: true, Installing: installing}
}

// InstallDlv installs Delve into Goi's tools dir (go install …@latest) in
// the background; progress arrives as debug.install events.
func (s *DebugService) InstallDlv() error {
	m, err := s.manager()
	if err != nil {
		return err
	}
	m.mu.Lock()
	if m.installing {
		m.mu.Unlock()
		return nil
	}
	m.installing = true
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	r, err := s.app.runner(ctx)
	if err != nil {
		cancel()
		m.mu.Lock()
		m.installing = false
		m.mu.Unlock()
		return err
	}
	s.app.appBus.Emit(events.TopicDebugInstall, DlvStatus{Installing: true, Message: "Installing Delve (go install " + DlvModule + ")…"})
	go func() {
		defer cancel()
		p, err := toolbin.Install(ctx, r.SDK.GoBin, r.Environ(), s.app.toolsDir(), DlvModule, "dlv")
		m.mu.Lock()
		m.installing = false
		m.mu.Unlock()
		if err != nil {
			s.app.log.Warn("install dlv", "err", err)
			s.app.appBus.Emit(events.TopicDebugInstall, DlvStatus{Message: err.Error()})
			return
		}
		s.app.log.Info("dlv installed", "path", p)
		s.app.appBus.Emit(events.TopicDebugInstall, DlvStatus{Path: p, Found: true, Message: "Delve installed"})
	}()
	return nil
}

// SetFileBreakpoints replaces a file's breakpoints (all of them, enabled or
// not) and applies them to running sessions.
func (s *DebugService) SetFileBreakpoints(path string, bps []LineBreakpoint) error {
	m, err := s.manager()
	if err != nil {
		return err
	}
	m.mu.Lock()
	if len(bps) == 0 {
		delete(m.lines, path)
	} else {
		m.lines[path] = append([]LineBreakpoint{}, bps...)
	}
	m.mu.Unlock()
	for _, ds := range m.active() {
		go func(ds *debugSession) {
			ctx, cancel := debugCtx()
			defer cancel()
			m.applyFile(ctx, ds, path, bps)
		}(ds)
	}
	return nil
}

// SetFunctionBreakpoints replaces function breakpoints and the
// break-on-panic flag, applying them to running sessions.
func (s *DebugService) SetFunctionBreakpoints(fns []FunctionBreakpoint, breakOnPanic bool) error {
	m, err := s.manager()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.funcs = append([]FunctionBreakpoint{}, fns...)
	m.panicBreak = breakOnPanic
	m.mu.Unlock()
	for _, ds := range m.active() {
		go func(ds *debugSession) {
			ctx, cancel := debugCtx()
			defer cancel()
			m.applyFunctions(ctx, ds)
		}(ds)
	}
	return nil
}

// Threads lists the goroutines of a stopped session.
func (s *DebugService) Threads(id string) ([]dap.Thread, error) {
	_, sess, err := s.get(id)
	if err != nil {
		return []dap.Thread{}, err
	}
	ctx, cancel := debugCtx()
	defer cancel()
	return sess.Threads(ctx)
}

// StackTrace returns frames of a goroutine (levels 0 = all).
func (s *DebugService) StackTrace(id string, threadID, start, levels int) (dap.StackTraceResponse, error) {
	_, sess, err := s.get(id)
	if err != nil {
		return dap.StackTraceResponse{StackFrames: []dap.StackFrame{}}, err
	}
	ctx, cancel := debugCtx()
	defer cancel()
	return sess.StackTrace(ctx, threadID, start, levels)
}

// Scopes returns the scopes of a frame.
func (s *DebugService) Scopes(id string, frameID int) ([]dap.Scope, error) {
	_, sess, err := s.get(id)
	if err != nil {
		return []dap.Scope{}, err
	}
	ctx, cancel := debugCtx()
	defer cancel()
	return sess.Scopes(ctx, frameID)
}

// Variables returns the children of a variables reference.
func (s *DebugService) Variables(id string, ref int) ([]dap.Variable, error) {
	_, sess, err := s.get(id)
	if err != nil {
		return []dap.Variable{}, err
	}
	ctx, cancel := debugCtx()
	defer cancel()
	return sess.Variables(ctx, ref)
}

// Evaluate evaluates an expression in a frame (context: watch, repl,
// hover, clipboard).
func (s *DebugService) Evaluate(id, expr string, frameID int, evalContext string) (dap.EvaluateResponse, error) {
	_, sess, err := s.get(id)
	if err != nil {
		return dap.EvaluateResponse{}, err
	}
	ctx, cancel := debugCtx()
	defer cancel()
	return sess.Evaluate(ctx, expr, frameID, evalContext)
}

// SetVariable changes a variable in the container ref.
func (s *DebugService) SetVariable(id string, ref int, name, value string) (dap.SetVariableResponse, error) {
	_, sess, err := s.get(id)
	if err != nil {
		return dap.SetVariableResponse{}, err
	}
	ctx, cancel := debugCtx()
	defer cancel()
	return sess.SetVariable(ctx, ref, name, value)
}

// ExceptionInfo describes the panic a goroutine stopped on.
func (s *DebugService) ExceptionInfo(id string, threadID int) (dap.ExceptionInfoResponse, error) {
	_, sess, err := s.get(id)
	if err != nil {
		return dap.ExceptionInfoResponse{}, err
	}
	ctx, cancel := debugCtx()
	defer cancel()
	return sess.ExceptionInfo(ctx, threadID)
}

// step runs a stepping request and marks the session running.
func (s *DebugService) step(id string, fn func(ctx context.Context, sess *debugger.Session) error) error {
	ds, sess, err := s.get(id)
	if err != nil {
		return err
	}
	ctx, cancel := debugCtx()
	defer cancel()
	ds.emit(DebugStateEvent{State: DebugRunning})
	if err := fn(ctx, sess); err != nil {
		ds.mu.Lock()
		tid := ds.threadID
		ds.mu.Unlock()
		ds.emit(DebugStateEvent{State: DebugStopped, ThreadID: tid, Reason: "error", Description: err.Error()})
		return err
	}
	return nil
}

// Continue resumes the program.
func (s *DebugService) Continue(id string, threadID int) error {
	return s.step(id, func(ctx context.Context, d *debugger.Session) error { return d.Continue(ctx, threadID) })
}

// Next steps over.
func (s *DebugService) Next(id string, threadID int) error {
	return s.step(id, func(ctx context.Context, d *debugger.Session) error { return d.Next(ctx, threadID) })
}

// StepIn steps into.
func (s *DebugService) StepIn(id string, threadID int) error {
	return s.step(id, func(ctx context.Context, d *debugger.Session) error { return d.StepIn(ctx, threadID) })
}

// StepOut steps out.
func (s *DebugService) StepOut(id string, threadID int) error {
	return s.step(id, func(ctx context.Context, d *debugger.Session) error { return d.StepOut(ctx, threadID) })
}

// RunToCursor continues to path:line.
func (s *DebugService) RunToCursor(id string, threadID int, path string, line int) error {
	return s.step(id, func(ctx context.Context, d *debugger.Session) error { return d.RunToCursor(ctx, threadID, path, line) })
}

// Pause pauses the program.
func (s *DebugService) Pause(id string, threadID int) error {
	_, sess, err := s.get(id)
	if err != nil {
		return err
	}
	ctx, cancel := debugCtx()
	defer cancel()
	return sess.Pause(ctx, threadID)
}

// Stop ends a session (a second call kills dlv).
func (s *DebugService) Stop(id string) error {
	m, err := s.manager()
	if err != nil {
		return err
	}
	m.mu.Lock()
	ds := m.sessions[id]
	m.mu.Unlock()
	if ds == nil {
		return s.app.runSvc.Stop(id)
	}
	ds.run.stop()
	return nil
}

// Sessions lists active debug session ids.
func (s *DebugService) Sessions() []string {
	m, err := s.manager()
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, ds := range m.active() {
		out = append(out, ds.id)
	}
	return out
}
