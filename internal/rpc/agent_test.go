package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/acp"
	"github.com/p4tin/goi/internal/agent"
	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/settings"
)

// agentHarness is a project window whose "frontend" answers agent.request
// events with respond.
type agentHarness struct {
	t    *testing.T
	a    *App
	w    *window
	root string
	rec  *events.Recorder

	mu       sync.Mutex
	respond  func(AgentRequestEvent) (any, error) // nil result and error: no answer
	requests []AgentRequestEvent
}

type harnessBus struct{ h *agentHarness }

func (b harnessBus) Emit(topic string, payload any) {
	b.h.rec.Emit(topic, payload)
	if ev, ok := payload.(AgentRequestEvent); ok && topic == events.TopicAgentRequest {
		go b.h.handle(ev)
	}
}

func (h *agentHarness) handle(ev AgentRequestEvent) {
	h.mu.Lock()
	h.requests = append(h.requests, ev)
	fn := h.respond
	h.mu.Unlock()
	if fn == nil {
		return
	}
	res, err := fn(ev)
	if res == nil && err == nil {
		return
	}
	ctx := withWindow(context.Background(), h.w)
	if err != nil {
		h.a.agentSvc.Respond(ctx, ev.ID, "", err.Error())
		return
	}
	b, _ := json.Marshal(res)
	h.a.agentSvc.Respond(ctx, ev.ID, string(b), "")
}

func (h *agentHarness) setRespond(fn func(AgentRequestEvent) (any, error)) {
	h.mu.Lock()
	h.respond = fn
	h.mu.Unlock()
}

func (h *agentHarness) seen(kind string) []AgentRequestEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []AgentRequestEvent
	for _, r := range h.requests {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

func (h *agentHarness) ctx() context.Context { return withWindow(context.Background(), h.w) }

// agentSetup opens a project whose settings define the fake agent "fake"
// (this test binary, see TestMain).
func agentSetup(t *testing.T, env ...string) *agentHarness {
	t.Helper()
	t.Setenv("GOI_HOME", t.TempDir())
	t.Setenv("GOI_FAKE_AGENT", "1")
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	root := module(t)
	a, err := NewApp(Options{ProjectPath: root})
	if err != nil {
		t.Fatal(err)
	}
	h := &agentHarness{t: t, a: a, root: root, rec: &events.Recorder{}}
	if err := a.start(context.Background(), harnessBus{h}, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Shutdown(context.Background()) })
	h.w = a.allWindows()[0]
	exe, _ := os.Executable()
	if err := h.w.settings.Set(settings.LayerUser, "ai.agents", map[string]any{"fake": agent.JoinCommandLine([]string{exe, "-test.run=^$"})}); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *agentHarness) newSession() AgentSessionInfo {
	h.t.Helper()
	info, err := h.a.agentSvc.NewSession(h.ctx(), "fake")
	if err != nil {
		h.t.Fatal(err)
	}
	return info
}

// turn sends a prompt and waits for its end; it returns the stop reason and
// the agent's message text.
func (h *agentHarness) turn(key, text string, blocks ...PromptBlock) (string, string) {
	h.t.Helper()
	before := len(h.rec.Events(events.TopicAgentUpdate))
	turns := len(h.rec.Events(events.TopicAgentTurn))
	if blocks == nil {
		blocks = []PromptBlock{{Type: "text", Text: text}}
	}
	if err := h.a.agentSvc.Prompt(h.ctx(), key, blocks); err != nil {
		h.t.Fatal(err)
	}
	ev := waitEvent(h.t, h.rec, events.TopicAgentTurn, turns)
	te := ev.Payload.(AgentTurnEvent)
	if te.Error != "" {
		h.t.Fatalf("%s: turn error %s", text, te.Error)
	}
	var sb strings.Builder
	for _, e := range h.rec.Events(events.TopicAgentUpdate)[before:] {
		u := e.Payload.(AgentUpdateEvent)
		var v struct {
			Kind    string `json:"sessionUpdate"`
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		json.Unmarshal(u.Update, &v)
		if u.Session == key && v.Kind == "agent_message_chunk" {
			sb.WriteString(v.Content.Text)
		}
	}
	return te.StopReason, sb.String()
}

// waitEvent waits for the topic's event number n (0-based).
func waitEvent(t *testing.T, rec *events.Recorder, topic string, n int) events.Event {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if evs := rec.Events(topic); len(evs) > n {
			return evs[n]
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %s event #%d", topic, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAgentSessionEndToEnd(t *testing.T) {
	h := agentSetup(t)
	file := filepath.Join(h.root, "a.go")
	h.setRespond(func(ev AgentRequestEvent) (any, error) {
		switch ev.Kind {
		case "fs.read":
			return acp.ReadTextFileResult{Content: "package unsaved"}, nil
		case "fs.write":
			return struct{}{}, nil
		case "permission":
			return acp.PermissionOutcome{Outcome: "selected", OptionID: "allow"}, nil
		case "tool":
			p := ev.Params.(map[string]any)
			return map[string]any{"text": fmt.Sprintf("%s for %s", p["name"], ev.Session)}, nil
		}
		return nil, fmt.Errorf("unexpected %s", ev.Kind)
	})

	info := h.newSession()
	if info.Key == "" || info.AgentName != "fake" || len(info.Modes) != 2 || info.CurrentMode != "ask" || !info.IDETools || !info.EmbeddedContext {
		t.Fatalf("session info = %+v", info)
	}
	key := info.Key
	if stop, out := h.turn(key, "echo hi there"); stop != acp.StopEndTurn || out != "hi there" {
		t.Errorf("echo: %s %q", stop, out)
	}
	if _, out := h.turn(key, "read "+file); out != "content: package unsaved" {
		t.Errorf("read: %q", out)
	}
	if r := h.seen("fs.read"); len(r) != 1 || r[0].Session != key {
		t.Errorf("fs.read requests = %+v", r)
	}
	// Paths outside the project never reach the frontend.
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if _, out := h.turn(key, "read "+outside); !strings.Contains(out, "inside the project") {
		t.Errorf("outside read: %q", out)
	}
	if n := len(h.seen("fs.read")); n != 1 {
		t.Errorf("outside read reached the frontend (%d fs.read)", n)
	}
	if _, out := h.turn(key, "write "+file+" package written"); out != "wrote "+file {
		t.Errorf("write: %q", out)
	}
	if w := h.seen("fs.write"); len(w) != 1 || !strings.Contains(fmt.Sprint(w[0].Params), "package written") {
		t.Errorf("fs.write = %+v", w)
	}
	if p := h.seen("permission"); len(p) != 1 {
		t.Errorf("permission requests = %d", len(p))
	}
	if _, out := h.turn(key, `mcp get_open_editors {}`); out != "mcp: get_open_editors for "+key {
		t.Errorf("mcp: %q", out)
	}
	if runtime.GOOS != "windows" {
		if _, out := h.turn(key, "run echo from-agent"); out != "exit 0: from-agent" {
			t.Errorf("run: %q", out)
		}
		var data string
		var exited bool
		for _, e := range h.rec.Events(events.TopicAgentTerminal) {
			te := e.Payload.(AgentTerminalEvent)
			data += te.Data
			exited = exited || te.Exit != nil
		}
		if !strings.Contains(data, "from-agent") || !exited {
			t.Errorf("terminal events: %q exited=%v", data, exited)
		}
	}

	// Mode switch arrives as an update.
	updates := len(h.rec.Events(events.TopicAgentUpdate))
	if err := h.a.agentSvc.SetMode(h.ctx(), key, "code"); err != nil {
		t.Fatal(err)
	}
	if u := waitEvent(t, h.rec, events.TopicAgentUpdate, updates).Payload.(AgentUpdateEvent); !strings.Contains(string(u.Update), "current_mode_update") {
		t.Errorf("mode update = %s", u.Update)
	}

	// The process is listed as running.
	list, _ := h.a.agentSvc.Agents(h.ctx())
	var fake AgentInfo
	for _, a := range list {
		if a.ID == "fake" {
			fake = a
		}
	}
	if !fake.Available || !fake.Running || fake.Source != agent.SourceSetting {
		t.Errorf("fake agent info = %+v", fake)
	}

	// Closing the project stops the agent.
	states := len(h.rec.Events(events.TopicAgentState))
	if err := h.w.closeProject(); err != nil {
		t.Fatal(err)
	}
	st := waitEvent(t, h.rec, events.TopicAgentState, states).Payload.(AgentStateEvent)
	if st.State != "stopped" || len(st.Sessions) != 1 || st.Sessions[0] != key {
		t.Errorf("state after close = %+v", st)
	}
}

func TestAgentCancelAnswersPendingPermission(t *testing.T) {
	h := agentSetup(t)
	h.setRespond(func(ev AgentRequestEvent) (any, error) {
		if ev.Kind == "fs.read" {
			return acp.ReadTextFileResult{Content: ""}, nil
		}
		return nil, nil // never answer the permission
	})
	key := h.newSession().Key
	turns := len(h.rec.Events(events.TopicAgentTurn))
	if err := h.a.agentSvc.Prompt(h.ctx(), key, []PromptBlock{{Type: "text", Text: "write " + filepath.Join(h.root, "b.go") + " x"}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "permission request", func() bool { return len(h.seen("permission")) == 1 })
	if err := h.a.agentSvc.Prompt(h.ctx(), key, []PromptBlock{{Type: "text", Text: "echo again"}}); err == nil {
		t.Error("a second prompt during a turn was accepted")
	}
	if err := h.a.agentSvc.Cancel(h.ctx(), key); err != nil {
		t.Fatal(err)
	}
	te := waitEvent(t, h.rec, events.TopicAgentTurn, turns).Payload.(AgentTurnEvent)
	if te.StopReason != acp.StopCancelled {
		t.Errorf("turn = %+v", te)
	}
}

func TestAgentWaitTurnCancelled(t *testing.T) {
	h := agentSetup(t)
	key := h.newSession().Key
	turns := len(h.rec.Events(events.TopicAgentTurn))
	h.a.agentSvc.Prompt(h.ctx(), key, []PromptBlock{{Type: "text", Text: "wait"}})
	time.Sleep(100 * time.Millisecond)
	h.a.agentSvc.Cancel(h.ctx(), key)
	if te := waitEvent(t, h.rec, events.TopicAgentTurn, turns).Payload.(AgentTurnEvent); te.StopReason != acp.StopCancelled {
		t.Errorf("turn = %+v", te)
	}
}

func TestAgentPromptContextBlocks(t *testing.T) {
	h := agentSetup(t)
	key := h.newSession().Key
	_, out := h.turn(key, "", PromptBlock{Type: "text", Text: "blocks"},
		PromptBlock{Type: "resource", URI: "file:///p/a.go", Text: "package a", MimeType: "text/x-go"},
		PromptBlock{Type: "resource_link", URI: "file:///p/b.go"})
	if out != "text | resource:file:///p/a.go=package a | resource_link:file:///p/b.go" {
		t.Errorf("blocks = %q", out)
	}
}

func TestAgentStdioMCPBridge(t *testing.T) {
	h := agentSetup(t, "GOI_FAKE_AGENT_STDIO_MCP=1")
	h.setRespond(func(ev AgentRequestEvent) (any, error) {
		return map[string]any{"text": "via bridge"}, nil
	})
	info := h.newSession()
	if !info.IDETools {
		t.Fatal("no IDE tools")
	}
	if _, out := h.turn(info.Key, "mcp git_status"); out != "mcp: via bridge" {
		t.Errorf("stdio mcp: %q", out)
	}
}

func TestAgentAuth(t *testing.T) {
	h := agentSetup(t, "GOI_FAKE_AGENT_AUTH=1")
	info := h.newSession()
	if !info.AuthRequired || info.Key != "" || len(info.AuthMethods) != 2 {
		t.Fatalf("info = %+v", info)
	}
	line, err := h.a.agentSvc.AuthCommand(h.ctx(), "fake", "term")
	if err != nil || !strings.HasSuffix(line, "--login") {
		t.Errorf("auth command = %q %v", line, err)
	}
	if err := h.a.agentSvc.Authenticate(h.ctx(), "fake", "login"); err != nil {
		t.Fatal(err)
	}
	if info := h.newSession(); info.AuthRequired || info.Key == "" {
		t.Errorf("after login = %+v", info)
	}
}

func TestAgentErrors(t *testing.T) {
	h := agentSetup(t)
	if _, err := h.a.agentSvc.NewSession(h.ctx(), "nope"); err == nil {
		t.Error("unknown agent accepted")
	}
	h.w.settings.Set(settings.LayerUser, "ai.agents", map[string]any{"broken": filepath.Join(t.TempDir(), "missing-agent")})
	if _, err := h.a.agentSvc.NewSession(h.ctx(), "broken"); err == nil {
		t.Error("missing program accepted")
	}
	if err := h.a.agentSvc.Prompt(h.ctx(), "s99", nil); err == nil {
		t.Error("prompt to an unknown session accepted")
	}
	if _, err := h.a.agentSvc.MCPConfig(h.ctx()); err == nil {
		t.Error("MCP config without the external server")
	}
}

func TestMCPRouting(t *testing.T) {
	h := agentSetup(t)
	host := h.a.mcp
	w, key, err := host.route(fmt.Sprintf("/%d/s3", h.w.id), nil)
	if err != nil || w != h.w || key != "s3" {
		t.Errorf("internal route = %v %q %v", w, key, err)
	}
	w, key, err = host.route("", json.RawMessage(`{"project":"`+filepath.ToSlash(filepath.Join(h.root, "pkg"))+`"}`))
	if err != nil || w != h.w || key != "" {
		t.Errorf("project route = %v %q %v", w, key, err)
	}
	if _, _, err := host.route("", json.RawMessage(`{"project":"/nowhere"}`)); err == nil {
		t.Error("unknown project routed")
	}
	if w, _, err := host.route("", nil); err != nil || w != h.w {
		t.Errorf("focused route = %v %v", w, err)
	}
	if _, _, err := host.route("/999/s1", nil); err == nil {
		t.Error("unknown window routed")
	}

	// The external server answers tools/list with the token.
	if err := host.startExternal(0); err != nil {
		t.Fatal(err)
	}
	cfg, err := h.a.agentSvc.MCPConfig(h.ctx())
	if err != nil || !strings.Contains(cfg.ClaudeCommand, cfg.Token) || !strings.Contains(cfg.JSON, cfg.URL) {
		t.Fatalf("config = %+v %v", cfg, err)
	}
	tok, _ := os.ReadFile(filepath.Join(h.a.configDir, "mcp-token"))
	if strings.TrimSpace(string(tok)) != cfg.Token {
		t.Error("token not persisted")
	}
}
