// Package mcp is a minimal Model Context Protocol server: the Streamable
// HTTP transport (stateless, JSON responses only) serving tools, and a stdio
// bridge to it. It knows nothing about Goi.
package mcp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Protocol versions this server accepts, newest first.
var Versions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Implementation names the server.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool describes one tool.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Content is one part of a tool result.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolResult is what a tool returns.
type ToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// TextResult is a result with one text part.
func TextResult(s string) ToolResult { return ToolResult{Content: []Content{{Type: "text", Text: s}}} }

// JSONResult is a result with v as indented JSON text.
func JSONResult(v any) (ToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ToolResult{}, err
	}
	return TextResult(string(b)), nil
}

// CallFunc runs a tool. target is what Options.Target extracted from the
// request (for routing). A returned error becomes a tool result with
// isError set, which is how MCP reports tool failures to the model.
type CallFunc func(ctx context.Context, target, name string, args json.RawMessage) (ToolResult, error)

// Options configures a Server.
type Options struct {
	Token string // required bearer token; empty disables the check
	Info  Implementation
	// Instructions is sent to clients at initialize.
	Instructions string
	Tools        []Tool
	Call         CallFunc
	// Target extracts a routing key from the request (e.g. from its path).
	Target func(*http.Request) string
}

// Server serves MCP over HTTP.
type Server struct {
	opts  Options
	names map[string]bool
}

// NewServer returns a Server for opts.
func NewServer(opts Options) *Server {
	names := map[string]bool{}
	for _, t := range opts.Tools {
		names[t.Name] = true
	}
	return &Server{opts: opts, names: names}
}

type request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// localOrigin reports whether an Origin header is absent or a loopback
// origin (guards against DNS rebinding from web pages).
func localOrigin(origin string) bool {
	if origin == "" || origin == "null" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// ServeHTTP implements the Streamable HTTP transport.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !localOrigin(r.Header.Get("Origin")) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	if s.opts.Token != "" {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.opts.Token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	target := ""
	if s.opts.Target != nil {
		target = s.opts.Target(r)
	}
	resp, ok := s.Handle(r.Context(), target, body)
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

// Handle processes one JSON-RPC message and returns the encoded response;
// ok is false for notifications and responses, which get none.
func (s *Server) Handle(ctx context.Context, target string, body []byte) (resp []byte, ok bool) {
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		return encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{CodeParseError, "parse error: " + err.Error()}}), true
	}
	if req.ID == nil {
		return nil, false // notification (or a response to us)
	}
	if req.Method == "" {
		return nil, false
	}
	result, rerr := s.dispatch(ctx, target, req.Method, req.Params)
	out := response{JSONRPC: "2.0", ID: *req.ID}
	if rerr != nil {
		out.Error = rerr
	} else {
		out.Result = result
	}
	return encode(out), true
}

func encode(r response) []byte {
	b, err := json.Marshal(r)
	if err != nil {
		b, _ = json.Marshal(response{JSONRPC: "2.0", ID: r.ID, Error: &rpcError{CodeInternalError, err.Error()}})
	}
	return b
}

func (s *Server) dispatch(ctx context.Context, target, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		v := Versions[0]
		if slices.Contains(Versions, p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		res := map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      s.opts.Info,
		}
		if s.opts.Instructions != "" {
			res["instructions"] = s.opts.Instructions
		}
		return res, nil
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		tools := s.opts.Tools
		if tools == nil {
			tools = []Tool{}
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{CodeInvalidParams, err.Error()}
		}
		if !s.names[p.Name] {
			return nil, &rpcError{CodeInvalidParams, "unknown tool: " + p.Name}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		if s.opts.Call == nil {
			return nil, &rpcError{CodeInternalError, "no tool handler"}
		}
		res, err := s.opts.Call(ctx, target, p.Name, p.Arguments)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, &rpcError{CodeInternalError, "cancelled"}
			}
			return ToolResult{Content: []Content{{Type: "text", Text: err.Error()}}, IsError: true}, nil
		}
		if res.Content == nil {
			res.Content = []Content{}
		}
		return res, nil
	}
	return nil, &rpcError{CodeMethodNotFound, fmt.Sprintf("method not found: %s", method)}
}
