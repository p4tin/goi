package rpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/mcp"
	"github.com/p4tin/goi/internal/paths"
)

// mcpListener is one running MCP HTTP server.
type mcpListener struct {
	srv   *http.Server
	base  string // http://127.0.0.1:port
	token string
}

// mcpHost serves the IDE tools over MCP: an internal server (random port and
// token) for agents Goi starts, and optionally an external one on a fixed
// port with a persistent token.
type mcpHost struct {
	app *App

	mu       sync.Mutex
	internal *mcpListener
	external *mcpListener
}

func randomToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *mcpHost) serve(addr, token string) (*mcpListener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := mcp.NewServer(mcp.Options{
		Token:        token,
		Info:         mcp.Implementation{Name: "goi", Version: paths.ProductVersion},
		Instructions: mcpInstructions,
		Tools:        agentTools(),
		Call:         h.call,
		Target:       func(r *http.Request) string { return strings.TrimPrefix(r.URL.Path, "/mcp") },
	})
	mux := http.NewServeMux()
	mux.Handle("/mcp", s)
	mux.Handle("/mcp/", s)
	l := &mcpListener{srv: &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}, base: "http://" + ln.Addr().String(), token: token}
	go func() {
		if err := l.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			h.app.log.Warn("mcp server", "err", err)
		}
	}()
	return l, nil
}

// internalListener starts the internal server on first use.
func (h *mcpHost) internalListener() (*mcpListener, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.internal != nil {
		return h.internal, nil
	}
	l, err := h.serve("127.0.0.1:0", randomToken())
	if err != nil {
		return nil, fmt.Errorf("mcp server: %w", err)
	}
	h.internal = l
	return l, nil
}

// externalToken reads (or creates) the persistent token for external clients.
func (h *mcpHost) externalToken() (string, error) {
	path := filepath.Join(h.app.configDir, "mcp-token")
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); len(t) >= 32 {
			return t, nil
		}
	}
	t := randomToken()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(t+"\n"), 0o600); err != nil {
		return "", err
	}
	return t, nil
}

// startExternal starts the external server on port.
func (h *mcpHost) startExternal(port int) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.external != nil {
		return nil
	}
	token, err := h.externalToken()
	if err != nil {
		return fmt.Errorf("mcp token: %w", err)
	}
	l, err := h.serve(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), token)
	if err != nil {
		return fmt.Errorf("mcp server on port %d: %w", port, err)
	}
	h.external = l
	h.app.log.Info("external mcp server", "url", l.base+"/mcp")
	return nil
}

func (h *mcpHost) externalListener() *mcpListener {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.external
}

func (h *mcpHost) close() {
	h.mu.Lock()
	ls := []*mcpListener{h.internal, h.external}
	h.internal, h.external = nil, nil
	h.mu.Unlock()
	for _, l := range ls {
		if l != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = l.srv.Shutdown(ctx)
			cancel()
		}
	}
}

// route resolves a tool call's window and session key. target is the URL
// path after /mcp: "/<windowID>/<sessionKey>" for agents Goi started, empty
// for external clients (the window whose project contains args.project, or
// the focused window).
func (h *mcpHost) route(target string, args json.RawMessage) (*window, string, error) {
	a := h.app
	parts := strings.Split(strings.Trim(target, "/"), "/")
	if parts[0] != "" {
		id, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("bad MCP path %q", target)
		}
		a.winMu.Lock()
		w := a.windows[uint(id)]
		a.winMu.Unlock()
		if w == nil {
			return nil, "", errWindowClosed
		}
		key := ""
		if len(parts) > 1 {
			key = parts[1]
		}
		return w, key, nil
	}
	var p struct {
		Project string `json:"project"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Project != "" {
		abs, err := filepath.Abs(p.Project)
		if err == nil {
			for _, w := range a.allWindows() {
				if s, err := w.current(); err == nil && (s.root == abs || within(s.root, abs)) {
					return w, "", nil
				}
			}
		}
		return nil, "", fmt.Errorf("no Goi window has %s open", p.Project)
	}
	if w := a.focusedWindow(); w != nil {
		return w, "", nil
	}
	return nil, "", errNoWindow
}

const toolTimeout = 15 * time.Minute

// call forwards a tool call to the window's frontend (frontend/src/agent/tools.ts).
func (h *mcpHost) call(ctx context.Context, target, name string, args json.RawMessage) (mcp.ToolResult, error) {
	w, key, err := h.route(target, args)
	if err != nil {
		return mcp.ToolResult{}, err
	}
	if _, err := w.current(); err != nil {
		return mcp.ToolResult{}, errors.New("the Goi window has no project open")
	}
	raw, err := w.ui.request(ctx, "tool", key, map[string]any{"name": name, "args": args}, toolTimeout)
	if err != nil {
		return mcp.ToolResult{}, err
	}
	var res struct {
		Text    *string `json:"text"`
		IsError bool    `json:"isError"`
	}
	if json.Unmarshal(raw, &res) == nil && res.Text != nil {
		r := mcp.TextResult(*res.Text)
		r.IsError = res.IsError
		return r, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return mcp.ToolResult{}, err
	}
	return mcp.JSONResult(v)
}

// bridgeExe is the program agents run for MCP over stdio (`goi mcp-bridge`).
func (a *App) bridgeExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, nil
}
