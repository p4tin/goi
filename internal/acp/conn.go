// Package acp is a minimal Agent Client Protocol (v1) client: JSON-RPC 2.0
// over newline-delimited JSON on a process's stdio, plus the protocol types
// Goi uses. It knows nothing about Goi.
package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
)

// JSON-RPC and ACP error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	CodeAuthRequired   = -32000
	CodeCancelled      = -32800
)

// Error is a JSON-RPC error, sent or received.
type Error struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("acp: %s (code %d)", e.Message, e.Code) }

// IsAuthRequired reports whether err is the agent's "authentication
// required" error.
func IsAuthRequired(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == CodeAuthRequired
}

// ErrClosed is returned by calls made after the connection closed.
var ErrClosed = errors.New("acp: connection closed")

// Handler answers requests and notifications sent by the peer. Requests run
// concurrently, each in its own goroutine; notifications run one at a time
// in arrival order (so streamed session updates stay ordered) and their
// result is ignored. A nil Handler replies MethodNotFound to every request.
type Handler func(ctx context.Context, method string, params json.RawMessage) (any, error)

type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
}

// response is written without omitempty on result: a successful reply must
// carry "result" even when it is null.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
}

type errorResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   *Error          `json:"error"`
}

type result struct {
	raw json.RawMessage
	err error
}

// Conn is a JSON-RPC 2.0 connection with one JSON message per line.
type Conn struct {
	r       io.Reader
	w       io.Writer
	closer  io.Closer
	handler Handler
	log     *slog.Logger

	ctx    context.Context // cancelled when the connection stops; handlers get it
	cancel context.CancelFunc

	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[string]chan result
	closed  bool
	err     error

	done chan struct{}
}

// NewConn starts reading from r. Close the Conn to stop it; closer (which
// may be nil) is closed then, which should end r.
func NewConn(r io.Reader, w io.Writer, closer io.Closer, h Handler, log *slog.Logger) *Conn {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Conn{r: r, w: w, closer: closer, handler: h, log: log, ctx: ctx, cancel: cancel, pending: map[string]chan result{}, done: make(chan struct{})}
	go c.read()
	return c
}

// Done is closed when the connection stops reading.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err returns why the connection stopped (nil while running or after a
// clean EOF).
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close closes the stream and fails pending calls.
func (c *Conn) Close() error {
	var err error
	if c.closer != nil {
		err = c.closer.Close()
	}
	<-c.done
	return err
}

// Call sends a request and decodes the result into out (which may be nil).
// If ctx ends first, the call is abandoned and ctx.Err() returned.
func (c *Conn) Call(ctx context.Context, method string, params, out any) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.nextID++
	id := strconv.FormatInt(c.nextID, 10)
	ch := make(chan result, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	rawID := json.RawMessage(id)
	p, err := marshalParams(params)
	if err != nil {
		c.forget(id)
		return err
	}
	if err := c.write(message{JSONRPC: "2.0", ID: &rawID, Method: method, Params: p}); err != nil {
		c.forget(id)
		return err
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if out == nil || len(r.raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(r.raw, out); err != nil {
			return fmt.Errorf("acp: decode %s result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.forget(id)
		return ctx.Err()
	}
}

func (c *Conn) forget(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Notify sends a notification.
func (c *Conn) Notify(method string, params any) error {
	p, err := marshalParams(params)
	if err != nil {
		return err
	}
	return c.write(message{JSONRPC: "2.0", Method: method, Params: p})
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("acp: encode params: %w", err)
	}
	return b, nil
}

func (c *Conn) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("acp: encode: %w", err)
	}
	b = append(b, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.w.Write(b); err != nil {
		return fmt.Errorf("acp: write: %w", err)
	}
	return nil
}

func (c *Conn) read() {
	r := bufio.NewReaderSize(c.r, 64*1024)
	var err error
	for {
		var line []byte
		line, err = r.ReadBytes('\n')
		if t := bytes.TrimSpace(line); len(t) > 0 {
			var m message
			if jerr := json.Unmarshal(t, &m); jerr != nil {
				c.log.Warn("acp: bad message", "err", jerr, "line", truncate(string(t), 200))
			} else {
				c.dispatch(m)
			}
		}
		if err != nil {
			break
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		err = nil
	}
	c.mu.Lock()
	c.closed = true
	c.err = err
	pending := c.pending
	c.pending = map[string]chan result{}
	c.mu.Unlock()
	c.cancel()
	for _, ch := range pending {
		ch <- result{err: ErrClosed}
	}
	close(c.done)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (c *Conn) dispatch(m message) {
	switch {
	case m.Method == "" && m.ID != nil: // response
		id := strings.Trim(string(*m.ID), `"`)
		c.mu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if ch == nil {
			return
		}
		if m.Error != nil {
			ch <- result{err: m.Error}
		} else {
			ch <- result{raw: m.Result}
		}
	case m.ID != nil: // request from the peer
		go c.answer(*m.ID, m.Method, m.Params)
	default: // notification, handled in order
		if c.handler != nil {
			if _, err := c.handler(c.ctx, m.Method, m.Params); err != nil {
				c.log.Debug("acp: notification handler", "method", m.Method, "err", err)
			}
		}
	}
}

func (c *Conn) answer(id json.RawMessage, method string, params json.RawMessage) {
	if c.handler == nil {
		c.replyError(id, &Error{Code: CodeMethodNotFound, Message: "method not found: " + method})
		return
	}
	res, err := c.handler(c.ctx, method, params)
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			e = &Error{Code: CodeInternalError, Message: err.Error()}
		}
		c.replyError(id, e)
		return
	}
	b, err := json.Marshal(res)
	if err != nil {
		c.replyError(id, &Error{Code: CodeInternalError, Message: err.Error()})
		return
	}
	if werr := c.write(response{JSONRPC: "2.0", ID: id, Result: b}); werr != nil {
		c.log.Debug("acp: reply", "method", method, "err", werr)
	}
}

func (c *Conn) replyError(id json.RawMessage, e *Error) {
	if err := c.write(errorResponse{JSONRPC: "2.0", ID: id, Error: e}); err != nil {
		c.log.Debug("acp: reply error", "err", err)
	}
}
