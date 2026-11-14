package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/p4tin/goi/internal/acp"
	"github.com/p4tin/goi/internal/agent"
	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/paths"
)

// AgentStateEvent is the payload of agent.state.
type AgentStateEvent struct {
	Agent string `json:"agent"`
	// State is starting, ready, stopped or error.
	State   string `json:"state"`
	Message string `json:"message"`
	// Sessions lists the session keys that ended (stopped/error).
	Sessions []string `json:"sessions"`
}

// AgentUpdateEvent is the payload of agent.update: one ACP session/update,
// forwarded as is.
type AgentUpdateEvent struct {
	Session string          `json:"session"`
	Update  json.RawMessage `json:"update"`
}

// AgentTurnEvent is the payload of agent.turn: a prompt turn ended.
type AgentTurnEvent struct {
	Session    string `json:"session"`
	StopReason string `json:"stopReason"`
	Error      string `json:"error"`
}

// AgentTerminalEvent is the payload of agent.terminal: output of a command
// an agent runs, and finally its exit status.
type AgentTerminalEvent struct {
	ID   string          `json:"id"`
	Data string          `json:"data"`
	Exit *acp.ExitStatus `json:"exit"`
}

// AgentInstallEvent is the payload of agent.install (app-wide).
type AgentInstallEvent struct {
	Agent string `json:"agent"`
	Line  string `json:"line"`
	Done  bool   `json:"done"`
	Error string `json:"error"`
}

// AgentInfo describes an agent for the agent picker.
type AgentInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	Available   bool   `json:"available"`
	// Source is setting, tools or path when available.
	Source string `json:"source"`
	// Command is the resolved command line, or why it isn't available.
	Command     string `json:"command"`
	Installable bool   `json:"installable"`
	Package     string `json:"package"`
	Running     bool   `json:"running"`
}

// AgentSessionInfo describes a new agent session.
type AgentSessionInfo struct {
	Key         string            `json:"key"`
	Agent       string            `json:"agent"`
	AgentName   string            `json:"agentName"`
	Modes       []acp.SessionMode `json:"modes"`
	CurrentMode string            `json:"currentMode"`
	// EmbeddedContext: the agent accepts file contents in prompts.
	EmbeddedContext bool `json:"embeddedContext"`
	// IDETools: the session got Goi's MCP server.
	IDETools     bool             `json:"ideTools"`
	AuthRequired bool             `json:"authRequired"`
	AuthMethods  []acp.AuthMethod `json:"authMethods"`
}

// PromptBlock is one part of a prompt: text, resource (a file's text) or
// resource_link (a file reference).
type PromptBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	URI      string `json:"uri"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
}

// AgentTerminal is a snapshot of an agent command's output.
type AgentTerminal struct {
	Output    string          `json:"output"`
	Truncated bool            `json:"truncated"`
	Exit      *acp.ExitStatus `json:"exit"`
}

// MCPConfig is how external MCP clients reach Goi.
type MCPConfig struct {
	URL           string `json:"url"`
	Token         string `json:"token"`
	JSON          string `json:"json"`
	ClaudeCommand string `json:"claudeCommand"`
}

// agentManager runs the agents of one project window.
type agentManager struct {
	app  *App
	win  *window
	sess *session

	startMu sync.Mutex // serializes process starts

	mu       sync.Mutex
	procs    map[string]*agentProc
	sessions map[string]*agentSess
	next     int
	closed   bool
}

// agentProc is a running agent process; it serves the agent's requests.
type agentProc struct {
	m      *agentManager
	id     string
	name   string
	client *agent.Client

	// Guarded by m.mu.
	bySID map[string]string            // ACP session id → session key
	early map[string][]json.RawMessage // updates for sessions not registered yet

	termMu   sync.Mutex
	batchers map[string]*outputBatcher

	stopping atomic.Bool // stopped on purpose (not an error)
}

// stop ends the agent process.
func (p *agentProc) stop() {
	p.stopping.Store(true)
	p.client.Close()
}

type agentSess struct {
	key    string
	sid    string
	proc   *agentProc
	inTurn bool
}

func newAgentManager(w *window, s *session) *agentManager {
	return &agentManager{app: w.app, win: w, sess: s, procs: map[string]*agentProc{}, sessions: map[string]*agentSess{}}
}

func (m *agentManager) close() {
	m.mu.Lock()
	m.closed = true
	procs := make([]*agentProc, 0, len(m.procs))
	for _, p := range m.procs {
		procs = append(procs, p)
	}
	m.mu.Unlock()
	for _, p := range procs {
		p.stop()
	}
}

func (m *agentManager) aiSettings() (overrides map[string]string, mcpOn bool) {
	ai := m.win.settings.Effective().AI
	return ai.Agents, ai.MCPServer
}

// definition finds an agent by id among the catalogue and custom agents.
func definition(id string, overrides map[string]string) (agent.Definition, bool) {
	for _, d := range agent.Definitions(overrides) {
		if d.ID == id {
			return d, true
		}
	}
	return agent.Definition{}, false
}

// env is the project's Go environment for agent processes and commands.
func (m *agentManager) env() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if r, err := m.win.runner(ctx); err == nil {
		return r.Environ()
	}
	return os.Environ()
}

// proc returns the running process for def, starting and initializing it
// if needed.
func (m *agentManager) proc(ctx context.Context, def agent.Definition, override string) (*agentProc, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errNoProject
	}
	if p := m.procs[def.ID]; p != nil {
		m.mu.Unlock()
		return p, nil
	}
	m.mu.Unlock()

	cmd, err := agent.Resolve(def, override, m.app.toolsDir(), nil)
	if err != nil {
		return nil, err
	}
	m.win.bus.Emit(events.TopicAgentState, AgentStateEvent{Agent: def.ID, State: "starting", Message: "Starting " + def.Name + "…", Sessions: []string{}})
	p := &agentProc{m: m, id: def.ID, name: def.Name, bySID: map[string]string{}, early: map[string][]json.RawMessage{}, batchers: map[string]*outputBatcher{}}
	c, err := agent.Start(agent.Options{
		Command:        cmd,
		Dir:            m.sess.root,
		Env:            m.env(),
		Host:           p,
		Log:            m.app.log,
		TerminalPrefix: def.ID + "-",
		ClientVersion:  paths.ProductVersion,
	})
	if err != nil {
		m.win.bus.Emit(events.TopicAgentState, AgentStateEvent{Agent: def.ID, State: "error", Message: err.Error(), Sessions: []string{}})
		return nil, err
	}
	p.client = c
	if _, err := c.Initialize(ctx); err != nil {
		c.Close()
		m.win.bus.Emit(events.TopicAgentState, AgentStateEvent{Agent: def.ID, State: "error", Message: err.Error(), Sessions: []string{}})
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		c.Close()
		return nil, errNoProject
	}
	m.procs[def.ID] = p
	m.mu.Unlock()
	m.app.log.Info("agent started", "agent", def.ID, "command", cmd.Path, "source", cmd.Source)
	m.win.bus.Emit(events.TopicAgentState, AgentStateEvent{Agent: def.ID, State: "ready", Message: def.Name + " is ready", Sessions: []string{}})
	go m.watch(p)
	return p, nil
}

// watch cleans up after the agent process exits.
func (m *agentManager) watch(p *agentProc) {
	<-p.client.Done()
	m.mu.Lock()
	if m.procs[p.id] == p {
		delete(m.procs, p.id)
	}
	var ended []string
	for k, s := range m.sessions {
		if s.proc == p {
			ended = append(ended, k)
			delete(m.sessions, k)
		}
	}
	m.mu.Unlock()
	sort.Strings(ended)
	if ended == nil {
		ended = []string{}
	}
	state, msg := "stopped", p.name+" stopped"
	if err := p.client.ExitError(); !p.stopping.Load() && err != nil && !strings.HasPrefix(err.Error(), "agent exited with code 0") {
		state, msg = "error", err.Error()
	}
	m.app.log.Info("agent exited", "agent", p.id, "state", state)
	m.win.bus.Emit(events.TopicAgentState, AgentStateEvent{Agent: p.id, State: state, Message: msg, Sessions: ended})
}

func (m *agentManager) mcpServers(p *agentProc, key string) ([]acp.MCPServer, bool) {
	_, on := m.aiSettings()
	if !on {
		return nil, false
	}
	l, err := m.app.mcp.internalListener()
	if err != nil {
		m.app.log.Warn("mcp server", "err", err)
		return nil, false
	}
	url := fmt.Sprintf("%s/mcp/%d/%s", l.base, m.win.id, key)
	if p.client.Info().AgentCapabilities.MCPCapabilities.HTTP {
		return []acp.MCPServer{{Name: "goi", URL: url, Headers: []acp.NameValue{{Name: "Authorization", Value: "Bearer " + l.token}}}}, true
	}
	exe, err := m.app.bridgeExe()
	if err != nil {
		m.app.log.Warn("mcp bridge", "err", err)
		return nil, false
	}
	return []acp.MCPServer{{Name: "goi", Command: exe, Args: []string{"mcp-bridge", "--url", url}, Env: []acp.NameValue{{Name: "GOI_MCP_TOKEN", Value: l.token}}}}, true
}

// newSession starts (or reuses) the agent and opens a session in the
// project root.
func (m *agentManager) newSession(ctx context.Context, agentID string) (AgentSessionInfo, error) {
	overrides, _ := m.aiSettings()
	def, ok := definition(agentID, overrides)
	if !ok {
		return AgentSessionInfo{}, fmt.Errorf("unknown agent %q", agentID)
	}
	p, err := m.proc(ctx, def, overrides[def.ID])
	if err != nil {
		return AgentSessionInfo{}, err
	}
	info := p.client.Info()
	out := AgentSessionInfo{Agent: def.ID, AgentName: def.Name, Modes: []acp.SessionMode{}, EmbeddedContext: info.AgentCapabilities.PromptCapabilities.EmbeddedContext, AuthMethods: info.AuthMethods}
	if out.AuthMethods == nil {
		out.AuthMethods = []acp.AuthMethod{}
	}
	if info.AgentInfo != nil && info.AgentInfo.Title != "" {
		out.AgentName = info.AgentInfo.Title
	}
	m.mu.Lock()
	m.next++
	key := fmt.Sprintf("s%d", m.next)
	m.mu.Unlock()
	servers, tools := m.mcpServers(p, key)
	ns, err := p.client.NewSession(ctx, m.sess.root, servers)
	if acp.IsAuthRequired(err) {
		out.AuthRequired = true
		return out, nil
	}
	if err != nil {
		return AgentSessionInfo{}, fmt.Errorf("%s: new session: %w", def.Name, err)
	}
	out.Key, out.IDETools = key, tools
	if ns.Modes != nil {
		out.Modes, out.CurrentMode = ns.Modes.AvailableModes, ns.Modes.CurrentModeID
		if out.Modes == nil {
			out.Modes = []acp.SessionMode{}
		}
	}
	m.mu.Lock()
	m.sessions[key] = &agentSess{key: key, sid: ns.SessionID, proc: p}
	p.bySID[ns.SessionID] = key
	early := p.early[ns.SessionID]
	delete(p.early, ns.SessionID)
	m.mu.Unlock()
	for _, u := range early {
		m.win.bus.Emit(events.TopicAgentUpdate, AgentUpdateEvent{Session: key, Update: u})
	}
	return out, nil
}

func (m *agentManager) session(key string) (*agentSess, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[key]
	if s == nil {
		return nil, fmt.Errorf("agent session %s has ended", key)
	}
	return s, nil
}

func toContentBlocks(blocks []PromptBlock) []acp.ContentBlock {
	out := make([]acp.ContentBlock, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "resource":
			out = append(out, acp.ContentBlock{Type: "resource", Resource: &acp.EmbeddedResource{URI: b.URI, Text: b.Text, MimeType: b.MimeType}})
		case "resource_link":
			name := b.Name
			if name == "" {
				name = filepath.Base(b.URI)
			}
			out = append(out, acp.ContentBlock{Type: "resource_link", URI: b.URI, Name: name, MimeType: b.MimeType})
		default:
			out = append(out, acp.TextBlock(b.Text))
		}
	}
	return out
}

// prompt starts a turn; agent.turn reports its end.
func (m *agentManager) prompt(key string, blocks []PromptBlock) error {
	m.mu.Lock()
	s := m.sessions[key]
	if s == nil {
		m.mu.Unlock()
		return fmt.Errorf("agent session %s has ended", key)
	}
	if s.inTurn {
		m.mu.Unlock()
		return errors.New("the agent is still working on the previous prompt")
	}
	s.inTurn = true
	m.mu.Unlock()
	go func() {
		stop, err := s.proc.client.Prompt(context.Background(), s.sid, toContentBlocks(blocks))
		m.mu.Lock()
		s.inTurn = false
		m.mu.Unlock()
		ev := AgentTurnEvent{Session: key, StopReason: stop}
		if err != nil {
			ev.Error = err.Error()
		}
		m.win.bus.Emit(events.TopicAgentTurn, ev)
	}()
	return nil
}

func (m *agentManager) cancel(key string) error {
	s, err := m.session(key)
	if err != nil {
		return err
	}
	m.win.ui.cancelSession(key)
	return s.proc.client.Cancel(s.sid)
}

func (m *agentManager) closeSession(key string) {
	m.mu.Lock()
	s := m.sessions[key]
	delete(m.sessions, key)
	if s != nil {
		delete(s.proc.bySID, s.sid)
	}
	m.mu.Unlock()
	if s == nil {
		return
	}
	m.win.ui.cancelSession(key)
	if s.inTurn {
		_ = s.proc.client.Cancel(s.sid)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.proc.client.CloseSession(ctx, s.sid)
	}()
}

func (m *agentManager) running(id string) *agentProc {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.procs[id]
}

// --- agent.Host

func (p *agentProc) key(sid string) string {
	p.m.mu.Lock()
	defer p.m.mu.Unlock()
	return p.bySID[sid]
}

func (p *agentProc) SessionUpdate(sid string, update json.RawMessage) {
	m := p.m
	m.mu.Lock()
	key := p.bySID[sid]
	if key == "" {
		// Sent before session/new returned to us; replayed on registration.
		if len(p.early[sid]) < 100 {
			p.early[sid] = append(p.early[sid], update)
		}
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	m.win.bus.Emit(events.TopicAgentUpdate, AgentUpdateEvent{Session: key, Update: update})
}

// checkPath allows absolute paths inside the project's content roots.
func (p *agentProc) checkPath(path string) error {
	if !filepath.IsAbs(path) {
		return &acp.Error{Code: acp.CodeInvalidParams, Message: "path must be absolute: " + path}
	}
	if _, ok := p.m.sess.contains(filepath.Clean(path)); !ok {
		return &acp.Error{Code: acp.CodeInvalidParams, Message: "Goi only lets agents access files inside the project: " + path}
	}
	return nil
}

const uiTimeout = 2 * time.Minute

func (p *agentProc) RequestPermission(ctx context.Context, params acp.RequestPermissionParams) (acp.PermissionOutcome, error) {
	key := p.key(params.SessionID)
	if key == "" {
		return acp.PermissionOutcome{Outcome: "cancelled"}, nil
	}
	if params.Options == nil {
		params.Options = []acp.PermissionOption{}
	}
	raw, err := p.m.win.ui.request(ctx, "permission", key, params, 0)
	if err != nil {
		return acp.PermissionOutcome{}, err
	}
	var out acp.PermissionOutcome
	if err := json.Unmarshal(raw, &out); err != nil || out.Outcome == "" {
		return acp.PermissionOutcome{Outcome: "cancelled"}, nil
	}
	return out, nil
}

func (p *agentProc) ReadTextFile(ctx context.Context, params acp.ReadTextFileParams) (string, error) {
	if err := p.checkPath(params.Path); err != nil {
		return "", err
	}
	raw, err := p.m.win.ui.request(ctx, "fs.read", p.key(params.SessionID), params, uiTimeout)
	if err != nil {
		return "", err
	}
	var res acp.ReadTextFileResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("read %s: %w", params.Path, err)
	}
	return res.Content, nil
}

func (p *agentProc) WriteTextFile(ctx context.Context, params acp.WriteTextFileParams) error {
	if err := p.checkPath(params.Path); err != nil {
		return err
	}
	_, err := p.m.win.ui.request(ctx, "fs.write", p.key(params.SessionID), params, uiTimeout)
	return err
}

func (p *agentProc) TerminalSpec(_ string, cwd string) (string, []string, error) {
	dir := p.m.sess.root
	if cwd != "" {
		if err := p.checkPath(cwd); err != nil {
			return "", nil, err
		}
		dir = cwd
	}
	return dir, p.m.env(), nil
}

func (p *agentProc) TerminalOutput(id string, data []byte, exit *acp.ExitStatus) {
	p.termMu.Lock()
	b := p.batchers[id]
	if b == nil && exit == nil {
		b = newOutputBatcher(func(chunks []OutputChunk) {
			var sb strings.Builder
			for _, c := range chunks {
				sb.WriteString(c.Text)
			}
			p.m.win.bus.Emit(events.TopicAgentTerminal, AgentTerminalEvent{ID: id, Data: sb.String()})
		})
		p.batchers[id] = b
	}
	if exit != nil {
		delete(p.batchers, id)
	}
	p.termMu.Unlock()
	if exit != nil {
		if b != nil {
			b.Close()
		}
		p.m.win.bus.Emit(events.TopicAgentTerminal, AgentTerminalEvent{ID: id, Exit: exit})
		return
	}
	b.Write("out", data)
}

// --- AgentService

// AgentService runs coding agents (Agent Client Protocol) for the project.
type AgentService struct {
	app        *App
	installing sync.Map // agent id → true while npm runs
}

func (s *AgentService) manager(ctx context.Context) (*agentManager, error) {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return nil, err
	}
	if sess.agents == nil {
		return nil, errNoProject
	}
	return sess.agents, nil
}

// Agents lists the catalogue and custom agents with their availability.
func (s *AgentService) Agents(ctx context.Context) ([]AgentInfo, error) {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return nil, err
	}
	m, _ := s.manager(ctx)
	overrides := w.settings.Effective().AI.Agents
	out := []AgentInfo{}
	for _, d := range agent.Definitions(overrides) {
		ai := AgentInfo{ID: d.ID, Name: d.Name, Description: d.Description, Homepage: d.Homepage, Installable: d.Package != "", Package: d.Package}
		if c, err := agent.Resolve(d, overrides[d.ID], s.app.toolsDir(), nil); err == nil {
			ai.Available, ai.Source = true, c.Source
			ai.Command = agent.JoinCommandLine(append([]string{c.Path}, c.Args...))
		} else {
			ai.Command = err.Error()
		}
		if m != nil && m.running(d.ID) != nil {
			ai.Running = true
		}
		out = append(out, ai)
	}
	return out, nil
}

// Install installs a catalogue agent's npm package into Goi's tools dir in
// the background; agent.install reports progress.
func (s *AgentService) Install(ctx context.Context, agentID string) error {
	def, ok := agent.Lookup(agentID)
	if !ok || def.Package == "" {
		return fmt.Errorf("%s can't be installed by Goi", agentID)
	}
	npm, err := agent.FindNpm(nil)
	if err != nil {
		return err
	}
	if _, busy := s.installing.LoadOrStore(agentID, true); busy {
		return fmt.Errorf("%s is already being installed", def.Name)
	}
	bus := s.app.appBus
	bus.Emit(events.TopicAgentInstall, AgentInstallEvent{Agent: agentID, Line: "npm install " + def.Package})
	go func() {
		defer s.installing.Delete(agentID)
		ictx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		err := agent.Install(ictx, def, s.app.toolsDir(), npm, os.Environ(), func(line string) {
			bus.Emit(events.TopicAgentInstall, AgentInstallEvent{Agent: agentID, Line: line})
		})
		ev := AgentInstallEvent{Agent: agentID, Done: true}
		if err != nil {
			ev.Error = err.Error()
			s.app.log.Warn("agent install", "agent", agentID, "err", err)
		} else {
			s.app.log.Info("agent installed", "agent", agentID)
		}
		bus.Emit(events.TopicAgentInstall, ev)
	}()
	return nil
}

// NewSession starts (or reuses) the agent and opens a session in the
// project root. When the agent needs a login, AuthRequired is set and the
// session isn't created; call Authenticate (or run the terminal method) and
// try again.
func (s *AgentService) NewSession(ctx context.Context, agentID string) (AgentSessionInfo, error) {
	m, err := s.manager(ctx)
	if err != nil {
		return AgentSessionInfo{}, err
	}
	// Starting npm-based agents can take a while; not tied to the call.
	nctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return m.newSession(nctx, agentID)
}

// Authenticate logs in with an agent-handled auth method.
func (s *AgentService) Authenticate(ctx context.Context, agentID, methodID string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	p := m.running(agentID)
	if p == nil {
		return fmt.Errorf("%s is not running", agentID)
	}
	actx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return p.client.Authenticate(actx, methodID)
}

// AuthCommand returns the command line to run in a terminal for a terminal
// auth method.
func (s *AgentService) AuthCommand(ctx context.Context, agentID, methodID string) (string, error) {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return "", err
	}
	m, err := s.manager(ctx)
	if err != nil {
		return "", err
	}
	p := m.running(agentID)
	if p == nil {
		return "", fmt.Errorf("%s is not running", agentID)
	}
	i := slices.IndexFunc(p.client.Info().AuthMethods, func(a acp.AuthMethod) bool { return a.ID == methodID })
	if i < 0 {
		return "", fmt.Errorf("unknown auth method %q", methodID)
	}
	method := p.client.Info().AuthMethods[i]
	overrides := w.settings.Effective().AI.Agents
	def, _ := definition(agentID, overrides)
	cmd, err := agent.Resolve(def, overrides[agentID], s.app.toolsDir(), nil)
	if err != nil {
		return "", err
	}
	line := agent.JoinCommandLine(append(append([]string{cmd.Path}, cmd.Args...), method.Args...))
	if len(method.Env) > 0 && runtime.GOOS != "windows" {
		var env []string
		for k, v := range method.Env {
			env = append(env, agent.JoinCommandLine([]string{k + "=" + v}))
		}
		sort.Strings(env)
		line = strings.Join(env, " ") + " " + line
	}
	return line, nil
}

// Prompt sends a prompt; it returns once sent. agent.update streams the
// answer and agent.turn reports the end of the turn.
func (s *AgentService) Prompt(ctx context.Context, key string, blocks []PromptBlock) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	return m.prompt(key, blocks)
}

// Cancel stops the session's running turn.
func (s *AgentService) Cancel(ctx context.Context, key string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	return m.cancel(key)
}

// SetMode switches the session's mode.
func (s *AgentService) SetMode(ctx context.Context, key, modeID string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	sess, err := m.session(key)
	if err != nil {
		return err
	}
	mctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return sess.proc.client.SetMode(mctx, sess.sid, modeID)
}

// CloseSession ends a session (the agent process keeps running).
func (s *AgentService) CloseSession(ctx context.Context, key string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	m.closeSession(key)
	return nil
}

// StopAgent stops an agent process and ends its sessions.
func (s *AgentService) StopAgent(ctx context.Context, agentID string) error {
	m, err := s.manager(ctx)
	if err != nil {
		return err
	}
	if p := m.running(agentID); p != nil {
		go p.stop()
	}
	return nil
}

// Respond answers an agent.request: result is JSON, or errMsg is set.
func (s *AgentService) Respond(ctx context.Context, id, result, errMsg string) error {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return err
	}
	r := uiReply{result: json.RawMessage(result)}
	if errMsg != "" {
		r = uiReply{err: errors.New(errMsg)}
	} else if !json.Valid(r.result) {
		r = uiReply{err: fmt.Errorf("invalid JSON answer for %s", id)}
	}
	w.ui.respond(id, r)
	return nil
}

// TerminalOutput returns an agent command's output so far.
func (s *AgentService) TerminalOutput(ctx context.Context, id string) (AgentTerminal, error) {
	m, err := s.manager(ctx)
	if err != nil {
		return AgentTerminal{}, err
	}
	agentID, _, _ := strings.Cut(id, "-")
	p := m.running(agentID)
	if p == nil {
		return AgentTerminal{}, agent.ErrNoTerminal
	}
	out, err := p.client.Terminals().Output(id)
	if err != nil {
		return AgentTerminal{}, err
	}
	return AgentTerminal{Output: out.Output, Truncated: out.Truncated, Exit: out.ExitStatus}, nil
}

// MCPConfig returns the external MCP server's address and ready-to-paste
// client configuration. It fails when ai.externalMcp is off.
func (s *AgentService) MCPConfig(ctx context.Context) (MCPConfig, error) {
	l := s.app.mcp.externalListener()
	if l == nil {
		return MCPConfig{}, errors.New("external MCP clients are off: turn on Settings › AI › External MCP Clients and restart Goi")
	}
	url := l.base + "/mcp"
	j, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"goi": map[string]any{
		"type": "http", "url": url, "headers": map[string]string{"Authorization": "Bearer " + l.token},
	}}}, "", "  ")
	return MCPConfig{
		URL:           url,
		Token:         l.token,
		JSON:          string(j),
		ClaudeCommand: fmt.Sprintf("claude mcp add --transport http goi %s --header \"Authorization: Bearer %s\"", url, l.token),
	}, nil
}
