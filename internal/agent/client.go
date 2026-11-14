package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/acp"
	"github.com/p4tin/goi/internal/process"
)

// Host serves the agent's requests to the IDE.
type Host interface {
	// SessionUpdate receives a session/update payload (raw).
	SessionUpdate(sessionID string, update json.RawMessage)
	// RequestPermission asks the user; it blocks until they answer or ctx
	// ends.
	RequestPermission(ctx context.Context, p acp.RequestPermissionParams) (acp.PermissionOutcome, error)
	ReadTextFile(ctx context.Context, p acp.ReadTextFileParams) (string, error)
	WriteTextFile(ctx context.Context, p acp.WriteTextFileParams) error
	// TerminalSpec validates a terminal's working directory ("" = default)
	// and returns it with the base environment for commands.
	TerminalSpec(sessionID, cwd string) (dir string, env []string, err error)
	// TerminalOutput sees each output chunk of an agent command and, last,
	// its exit status.
	TerminalOutput(id string, data []byte, exit *acp.ExitStatus)
}

// Options starts a Client.
type Options struct {
	Command Command
	Dir     string
	Env     []string
	Host    Host
	Log     *slog.Logger
	// TerminalPrefix makes terminal ids unique across clients.
	TerminalPrefix string
	ClientVersion  string
}

// Client is a running agent.
type Client struct {
	opts  Options
	proc  *process.Process
	conn  *acp.Conn
	terms *Terminals
	log   *slog.Logger

	mu     sync.Mutex
	stderr []byte
	info   acp.InitializeResult
}

const stderrKeep = 16 << 10

type procWriter struct{ p *process.Process }

func (w procWriter) Write(b []byte) (int, error) {
	if err := w.p.Write(b); err != nil {
		return 0, err
	}
	return len(b), nil
}

// Start runs the agent program. Call Initialize next.
func Start(o Options) (*Client, error) {
	if o.Host == nil {
		return nil, fmt.Errorf("agent: no host")
	}
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	c := &Client{opts: o, log: log}
	c.terms = NewTerminals(o.TerminalPrefix, o.Host.TerminalOutput)
	pr, pw := io.Pipe()
	p, err := process.Start(process.Spec{Path: o.Command.Path, Args: o.Command.Args, Dir: o.Dir, Env: o.Env, KillAfter: 2 * time.Second}, func(stream string, data []byte) {
		if stream == process.Stdout {
			_, _ = pw.Write(data)
			return
		}
		c.mu.Lock()
		c.stderr = append(c.stderr, data...)
		if n := len(c.stderr) - stderrKeep; n > 0 {
			c.stderr = append([]byte(nil), c.stderr[n:]...)
		}
		c.mu.Unlock()
	})
	if err != nil {
		pw.Close()
		return nil, fmt.Errorf("agent: start %s: %w", o.Command.Path, err)
	}
	c.proc = p
	c.conn = acp.NewConn(pr, procWriter{p}, pr, c.handle, log)
	go func() {
		<-p.Done()
		pw.Close()
	}()
	return c, nil
}

// Done is closed when the agent process exits.
func (c *Client) Done() <-chan struct{} { return c.proc.Done() }

// StderrTail is the end of what the agent wrote to stderr.
func (c *Client) StderrTail() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(strings.ToValidUTF8(string(c.stderr), "�"))
}

// ExitError describes how the process ended (call after Done).
func (c *Client) ExitError() error {
	info := c.proc.Wait()
	msg := fmt.Sprintf("agent exited with code %d", info.Code)
	if info.Signal != "" {
		msg = "agent killed by " + info.Signal
	}
	if info.Err != "" {
		msg += ": " + info.Err
	}
	if tail := c.StderrTail(); tail != "" {
		lines := strings.Split(tail, "\n")
		if len(lines) > 12 {
			lines = lines[len(lines)-12:]
		}
		msg += "\n" + strings.Join(lines, "\n")
	}
	return fmt.Errorf("%s", msg)
}

// Info is the agent's initialize result.
func (c *Client) Info() acp.InitializeResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.info
}

// call fails fast with the agent's exit message when it dies mid-call. Any
// transport failure counts (closed connection, or a write to an agent that
// already closed its stdin: EPIPE), not JSON-RPC errors or cancellation.
func (c *Client) call(ctx context.Context, method string, params, out any) error {
	err := c.conn.Call(ctx, method, params, out)
	var rpcErr *acp.Error
	if err != nil && ctx.Err() == nil && !errors.As(err, &rpcErr) {
		select {
		case <-c.proc.Done():
			return c.ExitError()
		case <-time.After(2 * time.Second):
		}
	}
	return err
}

// Initialize negotiates the protocol and capabilities.
func (c *Client) Initialize(ctx context.Context) (acp.InitializeResult, error) {
	var res acp.InitializeResult
	err := c.call(ctx, acp.MethodInitialize, acp.InitializeParams{
		ProtocolVersion:    acp.ProtocolVersion,
		ClientCapabilities: acp.ClientCapabilities{FS: acp.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}, Terminal: true},
		ClientInfo:         &acp.Implementation{Name: "goi", Title: "Goi", Version: c.opts.ClientVersion},
	}, &res)
	if err != nil {
		return res, fmt.Errorf("agent: initialize: %w", err)
	}
	if res.ProtocolVersion != acp.ProtocolVersion {
		return res, fmt.Errorf("agent: unsupported ACP protocol version %d", res.ProtocolVersion)
	}
	c.mu.Lock()
	c.info = res
	c.mu.Unlock()
	return res, nil
}

// Authenticate logs in with an agent-handled auth method.
func (c *Client) Authenticate(ctx context.Context, methodID string) error {
	return c.call(ctx, acp.MethodAuthenticate, acp.AuthenticateParams{MethodID: methodID}, nil)
}

// NewSession creates a session rooted at cwd.
func (c *Client) NewSession(ctx context.Context, cwd string, servers []acp.MCPServer) (acp.NewSessionResult, error) {
	if servers == nil {
		servers = []acp.MCPServer{}
	}
	var res acp.NewSessionResult
	err := c.call(ctx, acp.MethodSessionNew, acp.NewSessionParams{Cwd: cwd, MCPServers: servers}, &res)
	return res, err
}

// Prompt runs one turn and returns its stop reason.
func (c *Client) Prompt(ctx context.Context, sessionID string, blocks []acp.ContentBlock) (string, error) {
	var res acp.PromptResult
	if err := c.call(ctx, acp.MethodPrompt, acp.PromptParams{SessionID: sessionID, Prompt: blocks}, &res); err != nil {
		return "", err
	}
	return res.StopReason, nil
}

// Cancel asks the agent to stop the session's turn.
func (c *Client) Cancel(sessionID string) error {
	return c.conn.Notify(acp.MethodCancel, acp.SessionParams{SessionID: sessionID})
}

// SetMode switches the session's mode.
func (c *Client) SetMode(ctx context.Context, sessionID, modeID string) error {
	return c.call(ctx, acp.MethodSetMode, acp.SetModeParams{SessionID: sessionID, ModeID: modeID}, nil)
}

// CloseSession tells the agent the session is gone (best effort: agents
// without session/close support answer method-not-found).
func (c *Client) CloseSession(ctx context.Context, sessionID string) {
	_ = c.call(ctx, acp.MethodSessionClose, acp.SessionParams{SessionID: sessionID}, nil)
}

// Terminals are the agent's running commands.
func (c *Client) Terminals() *Terminals { return c.terms }

// Close ends the agent: stdin is closed (agents exit on EOF), then the
// process group is interrupted and killed if it doesn't exit.
func (c *Client) Close() {
	c.terms.CloseAll()
	_ = c.proc.CloseStdin()
	select {
	case <-c.proc.Done():
	case <-time.After(1500 * time.Millisecond):
		c.proc.Stop()
		<-c.proc.Done()
	}
	_ = c.conn.Close()
}

func decode[T any](params json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(params, &v); err != nil {
		return v, &acp.Error{Code: acp.CodeInvalidParams, Message: err.Error()}
	}
	return v, nil
}

func (c *Client) handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	h := c.opts.Host
	switch method {
	case acp.MethodSessionUpdate:
		n, err := decode[acp.SessionNotification](params)
		if err != nil {
			return nil, err
		}
		h.SessionUpdate(n.SessionID, n.Update)
		return nil, nil
	case acp.MethodRequestPermission:
		p, err := decode[acp.RequestPermissionParams](params)
		if err != nil {
			return nil, err
		}
		out, err := h.RequestPermission(ctx, p)
		if err != nil {
			return acp.RequestPermissionResult{Outcome: acp.PermissionOutcome{Outcome: "cancelled"}}, nil
		}
		return acp.RequestPermissionResult{Outcome: out}, nil
	case acp.MethodReadTextFile:
		p, err := decode[acp.ReadTextFileParams](params)
		if err != nil {
			return nil, err
		}
		s, err := h.ReadTextFile(ctx, p)
		if err != nil {
			return nil, err
		}
		return acp.ReadTextFileResult{Content: s}, nil
	case acp.MethodWriteTextFile:
		p, err := decode[acp.WriteTextFileParams](params)
		if err != nil {
			return nil, err
		}
		if err := h.WriteTextFile(ctx, p); err != nil {
			return nil, err
		}
		return struct{}{}, nil
	case acp.MethodTerminalCreate:
		p, err := decode[acp.CreateTerminalParams](params)
		if err != nil {
			return nil, err
		}
		dir, env, err := h.TerminalSpec(p.SessionID, p.Cwd)
		if err != nil {
			return nil, err
		}
		for _, e := range p.Env {
			env = append(env, e.Name+"="+e.Value)
		}
		var limit int64
		if p.OutputByteLimit != nil {
			limit = *p.OutputByteLimit
		}
		id, err := c.terms.Create(TerminalSpec{Command: p.Command, Args: p.Args, Dir: dir, Env: env, Limit: limit})
		if err != nil {
			return nil, err
		}
		return acp.CreateTerminalResult{TerminalID: id}, nil
	case acp.MethodTerminalOutput:
		p, err := decode[acp.TerminalParams](params)
		if err != nil {
			return nil, err
		}
		return c.terms.Output(p.TerminalID)
	case acp.MethodTerminalWait:
		p, err := decode[acp.TerminalParams](params)
		if err != nil {
			return nil, err
		}
		return c.terms.Wait(ctx, p.TerminalID)
	case acp.MethodTerminalKill:
		p, err := decode[acp.TerminalParams](params)
		if err != nil {
			return nil, err
		}
		return struct{}{}, c.terms.Kill(p.TerminalID)
	case acp.MethodTerminalRelease:
		p, err := decode[acp.TerminalParams](params)
		if err != nil {
			return nil, err
		}
		return struct{}{}, c.terms.Release(p.TerminalID)
	}
	return nil, &acp.Error{Code: acp.CodeMethodNotFound, Message: "method not found: " + method}
}
