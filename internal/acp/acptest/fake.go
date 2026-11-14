// Package acptest provides a scripted ACP agent for tests and E2E runs. The
// first word of each prompt picks what the agent does:
//
//	echo <text>           thought + agent message (in two chunks)
//	blocks                describes the prompt's content blocks
//	read <path>           fs/read_text_file, reports the content
//	write <path> <text>   asks permission, then fs/write_text_file
//	run <cmd> [args…]     terminal/create, waits, reports exit code and output
//	mcp <tool> [json]     calls a tool on the session's MCP server
//	plan                  sends a plan
//	commands              sends available commands (/hello)
//	wait                  runs until the turn is cancelled
//	fail                  answers the prompt with an error
//	(anything else)       echoes the text
//
// Environment: GOI_FAKE_AGENT_AUTH=1 requires authenticate("login") before
// session/new; GOI_FAKE_AGENT_STDIO_MCP=1 hides MCP-over-HTTP support.
package acptest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/p4tin/goi/internal/acp"
)

type fakeSession struct {
	cwd     string
	mcp     []acp.MCPServer
	mode    string
	cancel  context.CancelFunc
	turnCtx context.Context
}

type agent struct {
	conn *acp.Conn

	mu       sync.Mutex
	authed   bool
	sessions map[string]*fakeSession
	next     int
}

// Run serves one agent connection over in/out until in ends.
func Run(in io.Reader, out io.Writer) {
	a := &agent{sessions: map[string]*fakeSession{}}
	a.conn = acp.NewConn(in, out, nil, a.handle, nil)
	<-a.conn.Done()
}

// Main runs the fake agent on stdin/stdout; for test binaries that re-exec
// themselves as the agent.
func Main() {
	Run(os.Stdin, os.Stdout)
	os.Exit(0)
}

func (a *agent) session(id string) (*fakeSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessions[id]
	if s == nil {
		return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: "unknown session " + id}
	}
	return s, nil
}

func (a *agent) handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case acp.MethodInitialize:
		return acp.InitializeResult{
			ProtocolVersion: acp.ProtocolVersion,
			AgentCapabilities: acp.AgentCapabilities{
				PromptCapabilities: acp.PromptCapabilities{EmbeddedContext: true},
				MCPCapabilities:    acp.MCPCapabilities{HTTP: os.Getenv("GOI_FAKE_AGENT_STDIO_MCP") != "1"},
			},
			AuthMethods: []acp.AuthMethod{
				{ID: "login", Name: "Log in", Description: "Fake login"},
				{ID: "term", Name: "Log in from a terminal", Type: "terminal", Args: []string{"--login"}},
			},
			AgentInfo: &acp.Implementation{Name: "fake-agent", Version: "1.0"},
		}, nil
	case acp.MethodAuthenticate:
		var p acp.AuthenticateParams
		json.Unmarshal(params, &p)
		if p.MethodID != "login" {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: "unknown auth method"}
		}
		a.mu.Lock()
		a.authed = true
		a.mu.Unlock()
		return struct{}{}, nil
	case acp.MethodSessionNew:
		var p acp.NewSessionParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: err.Error()}
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if os.Getenv("GOI_FAKE_AGENT_AUTH") == "1" && !a.authed {
			return nil, &acp.Error{Code: acp.CodeAuthRequired, Message: "Authentication required"}
		}
		a.next++
		id := fmt.Sprintf("fake-%d", a.next)
		a.sessions[id] = &fakeSession{cwd: p.Cwd, mcp: p.MCPServers, mode: "ask"}
		return acp.NewSessionResult{SessionID: id, Modes: &acp.SessionModeState{
			CurrentModeID: "ask",
			AvailableModes: []acp.SessionMode{
				{ID: "ask", Name: "Ask", Description: "Ask before editing"},
				{ID: "code", Name: "Code", Description: "Edit freely"},
			},
		}}, nil
	case acp.MethodSetMode:
		var p acp.SetModeParams
		json.Unmarshal(params, &p)
		s, err := a.session(p.SessionID)
		if err != nil {
			return nil, err
		}
		s.mode = p.ModeID
		a.update(p.SessionID, map[string]any{"sessionUpdate": "current_mode_update", "currentModeId": p.ModeID})
		return struct{}{}, nil
	case acp.MethodCancel:
		var p acp.SessionParams
		json.Unmarshal(params, &p)
		a.mu.Lock()
		if s := a.sessions[p.SessionID]; s != nil && s.cancel != nil {
			s.cancel()
		}
		a.mu.Unlock()
		return nil, nil
	case acp.MethodSessionClose:
		var p acp.SessionParams
		json.Unmarshal(params, &p)
		a.mu.Lock()
		delete(a.sessions, p.SessionID)
		a.mu.Unlock()
		return struct{}{}, nil
	case acp.MethodPrompt:
		var p acp.PromptParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &acp.Error{Code: acp.CodeInvalidParams, Message: err.Error()}
		}
		return a.prompt(ctx, p)
	}
	return nil, &acp.Error{Code: acp.CodeMethodNotFound, Message: "method not found: " + method}
}

func (a *agent) update(sid string, u any) {
	b, _ := json.Marshal(u)
	_ = a.conn.Notify(acp.MethodSessionUpdate, acp.SessionNotification{SessionID: sid, Update: b})
}

func (a *agent) say(sid, text string) {
	a.update(sid, map[string]any{"sessionUpdate": "agent_message_chunk", "content": acp.TextBlock(text)})
}

func (a *agent) tool(sid string, u map[string]any) {
	a.update(sid, u)
}

func (a *agent) prompt(ctx context.Context, p acp.PromptParams) (any, error) {
	s, err := a.session(p.SessionID)
	if err != nil {
		return nil, err
	}
	tctx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	s.cancel, s.turnCtx = cancel, tctx
	a.mu.Unlock()
	defer cancel()

	text := ""
	for _, b := range p.Prompt {
		if b.Type == "text" {
			text = b.Text
			break
		}
	}
	sid := p.SessionID
	cmd, rest, _ := strings.Cut(strings.TrimSpace(text), " ")
	done := acp.PromptResult{StopReason: acp.StopEndTurn}
	switch cmd {
	case "echo":
		a.update(sid, map[string]any{"sessionUpdate": "agent_thought_chunk", "content": acp.TextBlock("thinking about " + rest)})
		half := len(rest) / 2
		a.say(sid, rest[:half])
		a.say(sid, rest[half:])
	case "blocks":
		var parts []string
		for _, b := range p.Prompt {
			d := b.Type
			if b.Resource != nil {
				d += ":" + b.Resource.URI + "=" + b.Resource.Text
			} else if b.URI != "" {
				d += ":" + b.URI
			}
			parts = append(parts, d)
		}
		a.say(sid, strings.Join(parts, " | "))
	case "read":
		a.tool(sid, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "r1", "title": "Read " + rest, "kind": "read", "status": "in_progress", "locations": []map[string]any{{"path": rest}}})
		var res acp.ReadTextFileResult
		if err := a.conn.Call(tctx, acp.MethodReadTextFile, acp.ReadTextFileParams{SessionID: sid, Path: rest}, &res); err != nil {
			a.tool(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "r1", "status": "failed"})
			a.say(sid, "read failed: "+err.Error())
			break
		}
		a.tool(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "r1", "status": "completed"})
		a.say(sid, "content: "+res.Content)
	case "write":
		path, body, _ := strings.Cut(rest, " ")
		var old acp.ReadTextFileResult
		_ = a.conn.Call(tctx, acp.MethodReadTextFile, acp.ReadTextFileParams{SessionID: sid, Path: path}, &old)
		diff := map[string]any{"type": "diff", "path": path, "oldText": old.Content, "newText": body}
		call := map[string]any{"toolCallId": "w1", "title": "Write " + path, "kind": "edit", "status": "pending", "content": []any{diff}, "locations": []map[string]any{{"path": path}}}
		a.tool(sid, merge(map[string]any{"sessionUpdate": "tool_call"}, call))
		tc, _ := json.Marshal(call)
		var perm acp.RequestPermissionResult
		err := a.conn.Call(tctx, acp.MethodRequestPermission, acp.RequestPermissionParams{SessionID: sid, ToolCall: tc, Options: []acp.PermissionOption{
			{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
			{OptionID: "reject", Name: "Reject", Kind: "reject_once"},
		}}, &perm)
		if err != nil || perm.Outcome.Outcome == "cancelled" || tctx.Err() != nil {
			a.tool(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "w1", "status": "failed"})
			return acp.PromptResult{StopReason: acp.StopCancelled}, nil
		}
		if perm.Outcome.OptionID != "allow" {
			a.tool(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "w1", "status": "failed"})
			a.say(sid, "rejected")
			break
		}
		if err := a.conn.Call(tctx, acp.MethodWriteTextFile, acp.WriteTextFileParams{SessionID: sid, Path: path, Content: body}, nil); err != nil {
			a.tool(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "w1", "status": "failed"})
			a.say(sid, "write failed: "+err.Error())
			break
		}
		a.tool(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "w1", "status": "completed"})
		a.say(sid, "wrote "+path)
	case "run":
		f := strings.Fields(rest)
		if len(f) == 0 {
			a.say(sid, "nothing to run")
			break
		}
		var ct acp.CreateTerminalResult
		if err := a.conn.Call(tctx, acp.MethodTerminalCreate, acp.CreateTerminalParams{SessionID: sid, Command: f[0], Args: f[1:]}, &ct); err != nil {
			a.say(sid, "run failed: "+err.Error())
			break
		}
		a.tool(sid, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "t1", "title": rest, "kind": "execute", "status": "in_progress", "content": []any{map[string]any{"type": "terminal", "terminalId": ct.TerminalID}}})
		tp := acp.TerminalParams{SessionID: sid, TerminalID: ct.TerminalID}
		var st acp.ExitStatus
		if err := a.conn.Call(tctx, acp.MethodTerminalWait, tp, &st); err != nil {
			_ = a.conn.Call(context.Background(), acp.MethodTerminalKill, tp, nil)
			_ = a.conn.Call(context.Background(), acp.MethodTerminalRelease, tp, nil)
			return acp.PromptResult{StopReason: acp.StopCancelled}, nil
		}
		var out acp.TerminalOutputResult
		_ = a.conn.Call(tctx, acp.MethodTerminalOutput, tp, &out)
		_ = a.conn.Call(tctx, acp.MethodTerminalRelease, tp, nil)
		a.tool(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t1", "status": "completed"})
		code := -1
		if st.ExitCode != nil {
			code = *st.ExitCode
		}
		a.say(sid, fmt.Sprintf("exit %d: %s", code, strings.TrimSpace(out.Output)))
	case "mcp":
		name, args, _ := strings.Cut(rest, " ")
		if args == "" {
			args = "{}"
		}
		res, err := callMCP(tctx, s.mcp, name, json.RawMessage(args))
		if err != nil {
			a.say(sid, "mcp failed: "+err.Error())
			break
		}
		a.say(sid, "mcp: "+res)
	case "plan":
		a.update(sid, map[string]any{"sessionUpdate": "plan", "entries": []map[string]any{
			{"content": "Read the code", "priority": "high", "status": "completed"},
			{"content": "Fix the bug", "priority": "medium", "status": "in_progress"},
		}})
		a.say(sid, "planned")
	case "commands":
		a.update(sid, map[string]any{"sessionUpdate": "available_commands_update", "availableCommands": []map[string]any{{"name": "hello", "description": "Say hello"}}})
		a.say(sid, "commands sent")
	case "wait":
		a.say(sid, "waiting")
		<-tctx.Done()
		done.StopReason = acp.StopCancelled
	case "fail":
		return nil, errors.New("fake failure")
	default:
		a.say(sid, text)
	}
	return done, nil
}

func merge(a, b map[string]any) map[string]any {
	maps.Copy(a, b)
	return a
}

// callMCP initializes and calls one tool on the first MCP server, over HTTP
// or by running its stdio command.
func callMCP(ctx context.Context, servers []acp.MCPServer, name string, args json.RawMessage) (string, error) {
	if len(servers) == 0 {
		return "", errors.New("no MCP server")
	}
	s := servers[0]
	msgs := []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "fake-agent", "version": "1"}}},
		{"jsonrpc": "2.0", "method": "notifications/initialized"},
		{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}},
	}
	var last []byte
	if s.URL != "" {
		for _, m := range msgs {
			b, _ := json.Marshal(m)
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			for _, h := range s.Headers {
				req.Header.Set(h.Name, h.Value)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", err
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 300 && resp.StatusCode != http.StatusAccepted {
				return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
			}
			last = body
		}
	} else {
		cmd := exec.CommandContext(ctx, s.Command, s.Args...)
		cmd.Env = os.Environ()
		for _, e := range s.Env {
			cmd.Env = append(cmd.Env, e.Name+"="+e.Value)
		}
		var in bytes.Buffer
		for _, m := range msgs {
			b, _ := json.Marshal(m)
			in.Write(append(b, '\n'))
		}
		cmd.Stdin = &in
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("stdio MCP: %w", err)
		}
		sc := bufio.NewScanner(bytes.NewReader(out))
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			if len(bytes.TrimSpace(sc.Bytes())) > 0 {
				last = append([]byte(nil), sc.Bytes()...)
			}
		}
	}
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *acp.Error `json:"error"`
	}
	if err := json.Unmarshal(last, &resp); err != nil {
		return "", fmt.Errorf("bad MCP reply %q: %w", last, err)
	}
	if resp.Error != nil {
		return "", resp.Error
	}
	var parts []string
	for _, c := range resp.Result.Content {
		parts = append(parts, c.Text)
	}
	out := strings.Join(parts, "")
	if resp.Result.IsError {
		return "error: " + out, nil
	}
	return out, nil
}
