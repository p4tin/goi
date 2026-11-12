package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/process"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/internal/vfs"
)

// Run states (run.state).
const (
	RunStarting = "starting"
	RunBuilding = "building"
	RunRunning  = "running"
	RunExited   = "exited"
	RunFailed   = "failed"
)

// Streams beyond process.Stdout/Stderr: Goi's own messages in a console.
const streamSystem = "system"

// maxTaskDepth bounds nested before-launch configurations.
const maxTaskDepth = 5

// RunInfo is the payload of run.state and the result of RunService.Runs.
type RunInfo struct {
	ID        string            `json:"id"`
	Config    runconfig.Config  `json:"config"`
	State     string            `json:"state"`
	Debug     bool              `json:"debug"`
	Exit      *process.ExitInfo `json:"exit"`
	ElapsedMs int64             `json:"elapsedMs"`
	Message   string            `json:"message"`
	Command   string            `json:"command"`
	Test      bool              `json:"test"`
}

// RunOutputEvent is the payload of run.output.
type RunOutputEvent struct {
	ID     string        `json:"id"`
	Chunks []OutputChunk `json:"chunks"`
}

// TestEvent is one `go test -json` (test2json) event.
type TestEvent struct {
	Action      string  `json:"action"`
	Package     string  `json:"package"`
	Test        string  `json:"test"`
	Elapsed     float64 `json:"elapsed"`
	Output      string  `json:"output"`
	ImportPath  string  `json:"importPath"`
	FailedBuild string  `json:"failedBuild"`
}

// RunTestEvent is the payload of run.test.
type RunTestEvent struct {
	ID     string      `json:"id"`
	Events []TestEvent `json:"events"`
}

// ContextRequest asks for a temporary configuration for a gutter marker or
// editor/tree context.
type ContextRequest struct {
	// Path is the file (or directory for kind "dir").
	Path string `json:"path"`
	// Kind is main, test, bench, fuzz, example, file or dir.
	Kind string `json:"kind"`
	// Name is the function name for main/test/bench/fuzz/example.
	Name string `json:"name"`
}

var errNoRun = errors.New("no such run")

// runManager owns the project's run configurations and launched runs.
type runManager struct {
	app     *App
	win     *window
	root    string
	store   *runconfig.Store
	tempDir string
	cancel  func()

	mu     sync.Mutex
	runs   map[string]*runInstance
	seq    int
	closed bool
}

type runInstance struct {
	m     *runManager
	id    string
	debug bool

	mu      sync.Mutex
	cfg     runconfig.Config
	state   string
	exit    *process.ExitInfo
	message string
	command string
	test    bool
	started time.Time
	ended   time.Time
	proc    *process.Process
	stopped bool
	done    chan struct{}
	out     *outputBatcher
	tests   *testBatcher
	// stopHook stops a debug session started for this run.
	stopHook func()
}

func startRuns(win *window, s *session, w vfs.Watcher) *runManager {
	a := win.app
	hash := s.manager.Project().Hash
	shared := filepath.Join(s.root, ".goi", "runConfigurations")
	local := filepath.Join(a.configDir, "workspaces", hash, "runConfigurations")
	store, err := runconfig.Open(shared, local)
	if err != nil {
		a.log.Warn("run configurations", "err", err)
		store, _ = runconfig.Open("", local)
	}
	for _, e := range store.LoadErrors() {
		a.log.Warn("run configuration skipped", "err", e)
	}
	m := &runManager{
		app:     a,
		win:     win,
		root:    s.root,
		store:   store,
		tempDir: filepath.Join(a.cacheDir, "run", hash),
		runs:    map[string]*runInstance{},
	}
	m.cancel = w.Subscribe(func(evs []vfs.Event) {
		for _, e := range evs {
			if within(shared, e.Path) || e.Path == shared {
				if err := store.Reload(); err != nil {
					a.log.Warn("reload run configurations", "err", err)
				}
				m.emitConfigs()
				return
			}
		}
	})
	return m
}

func (m *runManager) emitConfigs() {
	m.win.bus.Emit(events.TopicRunConfigs, m.configs())
}

func (m *runManager) configs() []runconfig.Config {
	list := m.store.List()
	out := make([]runconfig.Config, len(list))
	for i, c := range list {
		out[i] = c.Normalize()
	}
	return out
}

// close kills every run.
func (m *runManager) close() {
	m.mu.Lock()
	m.closed = true
	runs := make([]*runInstance, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	for _, r := range runs {
		r.kill()
	}
	for _, r := range runs {
		select {
		case <-r.waitDone():
		case <-time.After(3 * time.Second):
		}
	}
}

// planContext gathers what runconfig needs from the project SDK and settings.
func (m *runManager) planContext() (runconfig.Context, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := m.win.runner(ctx)
	if err != nil {
		return runconfig.Context{}, fmt.Errorf("Go SDK: %w", err)
	}
	g := m.win.settings.Effective().Go
	mod := ""
	switch g.Vendor {
	case "on":
		mod = "-mod=vendor"
	case "off":
		mod = "-mod=mod"
	}
	r.Dir = m.root
	return runconfig.Context{
		Root:    m.root,
		GoBin:   r.SDK.GoBin,
		Env:     r.Environ(),
		Tags:    g.Tags,
		ModFlag: mod,
		TempDir: m.tempDir,
		PackageDir: func(pkg string) (string, error) {
			c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			out, err := r.Command(c, "list", "-f", "{{.Dir}}", pkg).Output()
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(out)), nil
		},
	}, nil
}

// launch starts cfg in a new run, or in the run reuseID (rerun in place).
func (m *runManager) launch(cfg runconfig.Config, debug bool, reuseID string) (string, error) {
	cfg = cfg.Normalize()
	if errs := cfg.Validate(m.root); len(errs) > 0 {
		return "", fmt.Errorf("%s: %s", cfg.Name, strings.Join(errs, "; "))
	}
	if debug && !cfg.Debuggable() {
		return "", fmt.Errorf("%s configurations can't be debugged", cfg.Type)
	}
	if !debug && cfg.Type == runconfig.TypeRemote {
		return "", errors.New("Go Remote configurations can only be debugged")
	}
	if debug && cfg.Type != runconfig.TypeRemote {
		// Fail fast so the UI can offer to install Delve.
		if _, err := m.win.resolveDlv(); err != nil {
			return "", err
		}
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", errNoProject
	}
	inst := m.runs[reuseID]
	if inst == nil {
		m.seq++
		inst = &runInstance{m: m, id: "run-" + strconv.Itoa(m.seq)}
		m.runs[inst.id] = inst
	}
	m.mu.Unlock()

	// Stop a previous launch in this run before reusing it.
	inst.kill()
	<-inst.waitDone()

	inst.mu.Lock()
	inst.cfg, inst.debug = cfg, debug
	inst.state, inst.exit, inst.message, inst.command, inst.test = RunStarting, nil, "", "", cfg.Type == runconfig.TypeTest
	inst.started, inst.ended = time.Now(), time.Time{}
	inst.proc, inst.stopped, inst.stopHook = nil, false, nil
	inst.done = make(chan struct{})
	id := inst.id
	inst.out = newOutputBatcher(func(chunks []OutputChunk) {
		m.win.bus.Emit(events.TopicRunOutput, RunOutputEvent{ID: id, Chunks: chunks})
	})
	inst.tests = newTestBatcher(func(evs []TestEvent) {
		m.win.bus.Emit(events.TopicRunTest, RunTestEvent{ID: id, Events: evs})
	})
	inst.mu.Unlock()
	inst.emitState()
	go inst.execute()
	return inst.id, nil
}

func (m *runManager) get(id string) (*runInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.runs[id]; ok {
		return r, nil
	}
	return nil, errNoRun
}

func (r *runInstance) waitDone() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done == nil {
		c := make(chan struct{})
		close(c)
		return c
	}
	return r.done
}

func (r *runInstance) info() RunInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	end := r.ended
	if end.IsZero() {
		end = time.Now()
	}
	return RunInfo{
		ID: r.id, Config: r.cfg, State: r.state, Debug: r.debug, Exit: r.exit,
		ElapsedMs: end.Sub(r.started).Milliseconds(), Message: r.message, Command: r.command, Test: r.test,
	}
}

func (r *runInstance) emitState() {
	r.m.win.bus.Emit(events.TopicRunState, r.info())
}

func (r *runInstance) setState(state, message string) {
	r.mu.Lock()
	r.state, r.message = state, message
	r.mu.Unlock()
	r.emitState()
}

func (r *runInstance) system(text string) {
	r.mu.Lock()
	out := r.out
	r.mu.Unlock()
	out.Write(streamSystem, []byte(text))
}

func (r *runInstance) isStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

// stop interrupts the current step (a second stop kills).
func (r *runInstance) stop() {
	r.mu.Lock()
	r.stopped = true
	p, hook := r.proc, r.stopHook
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	if p != nil {
		p.Stop()
	}
}

func (r *runInstance) kill() {
	r.mu.Lock()
	r.stopped = true
	p, hook := r.proc, r.stopHook
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	if p != nil {
		p.Kill()
	}
}

// finish records the end of the run.
func (r *runInstance) finish(state, message string, exit *process.ExitInfo) {
	r.mu.Lock()
	r.state, r.message, r.exit, r.ended = state, message, exit, time.Now()
	out, tests := r.out, r.tests
	r.mu.Unlock()
	tests.Close()
	out.Close()
	r.emitState()
}

func (r *runInstance) execute() {
	r.mu.Lock()
	done := r.done
	cfg := r.cfg
	r.mu.Unlock()
	defer close(done)

	pctx, err := r.m.planContext()
	if err != nil {
		r.system(err.Error() + "\n")
		r.finish(RunFailed, err.Error(), nil)
		return
	}
	if err := r.runTasks(cfg, pctx, map[string]bool{strings.ToLower(cfg.Name): true}, 0); err != nil {
		if r.isStopped() {
			r.finish(RunExited, "Stopped", nil)
			return
		}
		r.system(err.Error() + "\n")
		r.finish(RunFailed, err.Error(), nil)
		return
	}
	if r.isStopped() {
		r.finish(RunExited, "Stopped", nil)
		return
	}
	if r.debug {
		if r.m.app.debugLaunch == nil {
			r.finish(RunFailed, "The debugger is not available", nil)
			return
		}
		r.m.app.debugLaunch(r, pctx)
		return
	}
	plan, err := cfg.MakePlan(pctx)
	if err == nil {
		err = r.m.ensureTempDir()
	}
	if err != nil {
		r.system(err.Error() + "\n")
		r.finish(RunFailed, err.Error(), nil)
		return
	}
	if plan.Build != nil {
		r.setState(RunBuilding, "")
		info, err := r.step(plan.Build, false)
		if err != nil || info.Code != 0 {
			if r.isStopped() {
				r.finish(RunExited, "Stopped", &info)
				return
			}
			msg := "Build failed"
			if err != nil {
				msg = err.Error()
				r.system(msg + "\n")
			}
			r.finish(RunFailed, msg, &info)
			return
		}
	}
	if plan.Exec == nil {
		r.system("Build finished\n")
		r.finish(RunExited, "Build finished", &process.ExitInfo{})
		return
	}
	if r.isStopped() {
		r.finish(RunExited, "Stopped", nil)
		return
	}
	r.mu.Lock()
	r.command = plan.Exec.String()
	r.mu.Unlock()
	r.setState(RunRunning, "")
	info, err := r.step(plan.Exec, plan.Test)
	if err != nil {
		r.system(err.Error() + "\n")
		r.finish(RunFailed, err.Error(), nil)
		return
	}
	r.finish(RunExited, "", &info)
}

// step runs one command to completion, streaming its output. test parses
// stdout as test2json.
func (r *runInstance) step(c *runconfig.Cmd, test bool) (process.ExitInfo, error) {
	r.system(c.String() + "\n")
	r.mu.Lock()
	out, tests := r.out, r.tests
	r.mu.Unlock()
	var lines lineSplitter
	write := func(stream string, b []byte) {
		if !test || stream != process.Stdout {
			out.Write(stream, b)
			return
		}
		for _, line := range lines.Write(b) {
			tests.Add(parseTestLine(line))
		}
	}
	p, err := process.Start(process.Spec{Path: c.Path, Args: c.Args, Env: c.Env, Dir: c.Dir, StdinFile: c.Stdin}, write)
	if err != nil {
		return process.ExitInfo{Code: -1}, err
	}
	r.mu.Lock()
	r.proc = p
	stopped := r.stopped
	r.mu.Unlock()
	if stopped {
		p.Stop()
	}
	info := p.Wait()
	if rest := lines.Rest(); rest != "" {
		tests.Add(parseTestLine(rest))
	}
	r.mu.Lock()
	r.proc = nil
	r.mu.Unlock()
	return info, nil
}

// parseTestLine decodes a test2json line; anything else becomes output.
func parseTestLine(line string) TestEvent {
	if strings.HasPrefix(line, "{") {
		var ev TestEvent
		if err := json.Unmarshal([]byte(line), &ev); err == nil && ev.Action != "" {
			return ev
		}
	}
	return TestEvent{Action: "output", Output: line + "\n"}
}

// runTasks runs cfg's before-launch tasks in order.
func (r *runInstance) runTasks(cfg runconfig.Config, pctx runconfig.Context, visiting map[string]bool, depth int) error {
	for i, t := range cfg.BeforeLaunch {
		if r.isStopped() {
			return errors.New("stopped")
		}
		label := fmt.Sprintf("Before launch %d", i+1)
		switch t.Type {
		case runconfig.TaskConfig:
			sub, ok := r.m.store.Get(t.Config)
			if !ok {
				return fmt.Errorf("%s: no configuration named %q", label, t.Config)
			}
			key := strings.ToLower(sub.Name)
			if visiting[key] || depth >= maxTaskDepth {
				return fmt.Errorf("%s: configuration %q runs itself", label, sub.Name)
			}
			visiting[key] = true
			if err := r.runTasks(sub, pctx, visiting, depth+1); err != nil {
				return err
			}
			delete(visiting, key)
			plan, err := sub.MakePlan(pctx)
			if err != nil {
				return fmt.Errorf("%s (%s): %w", label, sub.Name, err)
			}
			for _, c := range []*runconfig.Cmd{plan.Build, plan.Exec} {
				if c == nil {
					continue
				}
				info, err := r.step(c, false)
				if err != nil {
					return fmt.Errorf("%s (%s): %w", label, sub.Name, err)
				}
				if info.Code != 0 {
					return fmt.Errorf("%s (%s) failed with exit code %d", label, sub.Name, info.Code)
				}
			}
		case runconfig.TaskGenerate:
			pkg := t.Package
			if pkg == "" {
				pkg = "./..."
			}
			args := []string{"generate"}
			if len(pctx.Tags) > 0 {
				args = append(args, "-tags="+strings.Join(pctx.Tags, ","))
			}
			info, err := r.step(&runconfig.Cmd{Path: pctx.GoBin, Args: append(args, pkg), Dir: r.m.root, Env: pctx.Env}, false)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			if info.Code != 0 {
				return fmt.Errorf("%s: go generate failed with exit code %d", label, info.Code)
			}
		case runconfig.TaskCommand:
			args, err := runconfig.SplitArgs(t.Command)
			if err != nil || len(args) == 0 {
				return fmt.Errorf("%s: invalid command", label)
			}
			info, err := r.step(&runconfig.Cmd{Path: args[0], Args: args[1:], Dir: r.m.root, Env: pctx.Env}, false)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			if info.Code != 0 {
				return fmt.Errorf("%s: command failed with exit code %d", label, info.Code)
			}
		}
	}
	return nil
}

// contextConfig builds the temporary configuration for a context request.
func (m *runManager) contextConfig(req ContextRequest) (runconfig.Config, error) {
	path := req.Path
	if path == "" {
		return runconfig.Config{}, errors.New("no file")
	}
	dir := path
	if req.Kind != "dir" {
		dir = filepath.Dir(path)
	}
	rel := m.relDir(dir)
	c := runconfig.Config{Type: runconfig.TypeTest, Kind: runconfig.KindFunction, Directory: rel}
	switch req.Kind {
	case "main":
		c = runconfig.Config{Name: "go build " + rel, Type: runconfig.TypeApp, Kind: runconfig.KindPackage, Package: rel}
	case "test", "example", "fuzz":
		c.Name = req.Name + " in " + rel
		c.Pattern = "^" + req.Name + "$"
	case "bench":
		c.Name = req.Name + " in " + rel
		c.Pattern = "^$"
		c.Bench = "^" + req.Name + "$"
	case "file":
		c.Name = filepath.Base(path)
		c.Kind = runconfig.KindFile
		c.Files = []string{m.relPath(path)}
		c.Directory = ""
	case "dir":
		c.Name = "go test " + rel
		c.Kind = runconfig.KindDirectory
	default:
		return runconfig.Config{}, fmt.Errorf("unknown context kind %q", req.Kind)
	}
	return m.store.AddTemporary(c)
}

// relDir renders dir relative to the root as "./x/y" (or ".").
func (m *runManager) relDir(dir string) string {
	rel, err := filepath.Rel(m.root, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return dir
	}
	if rel == "." {
		return "."
	}
	return "./" + filepath.ToSlash(rel)
}

func (m *runManager) relPath(p string) string {
	rel, err := filepath.Rel(m.root, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return filepath.ToSlash(rel)
}

func (m *runManager) benchPath() string {
	return filepath.Join(filepath.Dir(m.store.LocalDir()), "bench.json")
}

// testBatcher coalesces test events like outputBatcher does for text.
type testBatcher struct {
	emit   func([]TestEvent)
	mu     sync.Mutex
	evs    []TestEvent
	timer  *time.Timer
	closed bool
}

const maxBatchEvents = 500

func newTestBatcher(emit func([]TestEvent)) *testBatcher { return &testBatcher{emit: emit} }

func (b *testBatcher) Add(ev TestEvent) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.evs = append(b.evs, ev)
	if len(b.evs) >= maxBatchEvents {
		out := b.take()
		b.mu.Unlock()
		b.emit(out)
		return
	}
	if b.timer == nil {
		b.timer = time.AfterFunc(flushInterval, b.Flush)
	}
	b.mu.Unlock()
}

func (b *testBatcher) take() []TestEvent {
	out := b.evs
	b.evs = nil
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	return out
}

func (b *testBatcher) Flush() {
	b.mu.Lock()
	out := b.take()
	b.mu.Unlock()
	if len(out) > 0 {
		b.emit(out)
	}
}

func (b *testBatcher) Close() {
	b.Flush()
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
}

// RunService manages run configurations and runs for the open project.
type RunService struct {
	app *App
}

func (s *RunService) manager(ctx context.Context) (*runManager, error) {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return nil, err
	}
	if sess.runs == nil {
		return nil, errNoProject
	}
	return sess.runs, nil
}

// Configs lists the project's run configurations.
func (s *RunService) Configs(ctx context.Context) []runconfig.Config {
	m, err := s.manager(ctx)
	if err != nil {
		return []runconfig.Config{}
	}
	return m.configs()
}

// SaveConfig creates or updates a configuration (oldName "" creates).
func (s *RunService) SaveConfig(ctx context.Context, c runconfig.Config, oldName string) (runconfig.Config, error) {
	m, err := s.manager(ctx)
	if err != nil {
		return runconfig.Config{}, err
	}
	saved, err := m.store.Save(c, oldName)
	if err != nil {
		return runconfig.Config{}, err
	}
	m.emitConfigs()
	return saved.Normalize(), nil
}

// DeleteConfig removes a configuration.
func (s *RunService) DeleteConfig(ctx context.Context, name string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	if err := m.store.Delete(name); err != nil {
		return err
	}
	m.emitConfigs()
	return nil
}

// Validate returns the configuration's problems (empty when runnable).
func (s *RunService) Validate(ctx context.Context, c runconfig.Config) []string {
	m, err := s.manager(ctx)
	if err != nil {
		return []string{err.Error()}
	}
	errs := c.Validate(m.root)
	if errs == nil {
		errs = []string{}
	}
	return errs
}

// ContextConfig creates (or refreshes) the temporary configuration for a
// gutter marker or context target and returns it.
func (s *RunService) ContextConfig(ctx context.Context, req ContextRequest) (runconfig.Config, error) {
	m, err := s.manager(ctx)
	if err != nil {
		return runconfig.Config{}, err
	}
	c, err := m.contextConfig(req)
	if err != nil {
		return runconfig.Config{}, err
	}
	m.emitConfigs()
	return c.Normalize(), nil
}

// AddTemporary stores c as a temporary configuration.
func (s *RunService) AddTemporary(ctx context.Context, c runconfig.Config) (runconfig.Config, error) {
	m, err := s.manager(ctx)
	if err != nil {
		return runconfig.Config{}, err
	}
	saved, err := m.store.AddTemporary(c)
	if err != nil {
		return runconfig.Config{}, err
	}
	m.emitConfigs()
	return saved.Normalize(), nil
}

// Run launches the named configuration and returns the run id.
func (s *RunService) Run(ctx context.Context, name string, debug bool) (string, error) {
	m, err := s.manager(ctx)
	if err != nil {
		return "", err
	}
	c, ok := m.store.Get(name)
	if !ok {
		return "", fmt.Errorf("no configuration named %q", name)
	}
	return m.launch(c, debug, "")
}

// RunConfig launches an unsaved configuration (test reruns), reusing the
// run reuseID when given.
func (s *RunService) RunConfig(ctx context.Context, c runconfig.Config, reuseID string, debug bool) (string, error) {
	m, err := s.manager(ctx)
	if err != nil {
		return "", err
	}
	return m.launch(c, debug, reuseID)
}

// Rerun launches the run's configuration again in the same run.
func (s *RunService) Rerun(ctx context.Context, id string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	r, err := m.get(id)
	if err != nil {
		return err
	}
	info := r.info()
	cfg := info.Config
	// Pick up edits to a saved configuration.
	if saved, ok := m.store.Get(cfg.Name); ok && len(cfg.Packages) == 0 {
		cfg = saved
	}
	_, err = m.launch(cfg, info.Debug, id)
	return err
}

// Stop stops a run: interrupt first, kill on a second call.
func (s *RunService) Stop(ctx context.Context, id string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	r, err := m.get(id)
	if err != nil {
		return err
	}
	r.stop()
	return nil
}

// Close stops a run and forgets it (its tab was closed).
func (s *RunService) Close(ctx context.Context, id string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	r, err := m.get(id)
	if err != nil {
		return err
	}
	r.kill()
	m.mu.Lock()
	delete(m.runs, id)
	m.mu.Unlock()
	return nil
}

// Input writes text to the run's stdin.
func (s *RunService) Input(ctx context.Context, id, text string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	r, err := m.get(id)
	if err != nil {
		return err
	}
	r.mu.Lock()
	p := r.proc
	r.mu.Unlock()
	if p == nil {
		return errors.New("the process is not running")
	}
	return p.Write([]byte(text))
}

// Runs lists the runs (running and finished) of this window.
func (s *RunService) Runs(ctx context.Context) []RunInfo {
	m, err := s.manager(ctx)
	if err != nil {
		return []RunInfo{}
	}
	m.mu.Lock()
	runs := make([]*runInstance, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.Unlock()
	out := make([]RunInfo, 0, len(runs))
	for _, r := range runs {
		out = append(out, r.info())
	}
	return out
}

// Active counts runs that are still starting, building or running.
func (s *RunService) Active(ctx context.Context) int {
	n := 0
	for _, r := range s.Runs(ctx) {
		if r.State != RunExited && r.State != RunFailed {
			n++
		}
	}
	return n
}

// LoadBench returns the saved benchmark history JSON ("" if none).
func (s *RunService) LoadBench(ctx context.Context) (string, error) {
	m, err := s.manager(ctx)
	if err != nil {
		return "", err
	}
	return readFileOrEmpty(m.benchPath())
}

// SaveBench stores the benchmark history JSON.
func (s *RunService) SaveBench(ctx context.Context, data string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	if !json.Valid([]byte(data)) {
		return errors.New("invalid benchmark history")
	}
	return writeFileAtomic(m.benchPath(), []byte(data))
}

// ensureTempDir makes sure the run binaries directory exists.
func (m *runManager) ensureTempDir() error { return os.MkdirAll(m.tempDir, 0o755) }
