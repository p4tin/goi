package lsp

import (
	"bufio"
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

// JSON-RPC error codes used by Goi.
const (
	CodeMethodNotFound   = -32601
	CodeInternalError    = -32603
	CodeRequestCancelled = -32800
)

// ResponseError is a JSON-RPC error returned by the peer.
type ResponseError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

func (e *ResponseError) Error() string { return fmt.Sprintf("lsp: %s (code %d)", e.Message, e.Code) }

// ErrClosed is returned by calls made after the connection closed.
var ErrClosed = errors.New("lsp: connection closed")

// Handler answers requests and notifications sent by the peer. For
// notifications the result is ignored. A nil Handler replies
// MethodNotFound to every request.
type Handler func(ctx context.Context, method string, params json.RawMessage) (any, error)

type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *ResponseError   `json:"error,omitempty"`
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
	Error   *ResponseError  `json:"error"`
}

type result struct {
	raw json.RawMessage
	err error
}

// Conn is a JSON-RPC 2.0 connection using LSP base-protocol framing.
type Conn struct {
	rwc     io.ReadWriteCloser
	handler Handler
	log     *slog.Logger

	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[string]chan result
	closed  bool
	err     error

	done chan struct{}
}

// NewConn starts reading from rwc. Close the Conn (or rwc) to stop it.
func NewConn(rwc io.ReadWriteCloser, h Handler, log *slog.Logger) *Conn {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	c := &Conn{rwc: rwc, handler: h, log: log, pending: map[string]chan result{}, done: make(chan struct{})}
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

// Close closes the underlying stream and fails pending calls.
func (c *Conn) Close() error {
	err := c.rwc.Close()
	<-c.done
	return err
}

// Call sends a request and decodes the result into out (which may be nil).
// If ctx ends first, $/cancelRequest is sent and ctx.Err() returned.
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
			return fmt.Errorf("lsp: decode %s result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.forget(id)
		_ = c.Notify(context.Background(), "$/cancelRequest", map[string]any{"id": c.nextIDValue(id)})
		return ctx.Err()
	}
}

func (c *Conn) nextIDValue(id string) any {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return id
	}
	return n
}

func (c *Conn) forget(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Notify sends a notification.
func (c *Conn) Notify(_ context.Context, method string, params any) error {
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
		return nil, fmt.Errorf("lsp: encode params: %w", err)
	}
	return b, nil
}

func (c *Conn) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("lsp: encode: %w", err)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := fmt.Fprintf(c.rwc, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return fmt.Errorf("lsp: write: %w", err)
	}
	if _, err := c.rwc.Write(b); err != nil {
		return fmt.Errorf("lsp: write: %w", err)
	}
	return nil
}

func (c *Conn) read() {
	r := bufio.NewReaderSize(c.rwc, 64*1024)
	var err error
	for {
		var body []byte
		body, err = readFrame(r)
		if err != nil {
			break
		}
		var m message
		if jerr := json.Unmarshal(body, &m); jerr != nil {
			c.log.Warn("lsp: bad message", "err", jerr)
			continue
		}
		c.dispatch(m)
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
	for _, ch := range pending {
		ch <- result{err: ErrClosed}
	}
	close(c.done)
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, val, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil {
				return nil, fmt.Errorf("lsp: bad Content-Length %q", val)
			}
			length = n
		}
	}
	if length < 0 {
		return nil, errors.New("lsp: missing Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
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
	default: // notification
		if c.handler != nil {
			go func() {
				if _, err := c.handler(context.Background(), m.Method, m.Params); err != nil {
					c.log.Debug("lsp: notification handler", "method", m.Method, "err", err)
				}
			}()
		}
	}
}

func (c *Conn) answer(id json.RawMessage, method string, params json.RawMessage) {
	if c.handler == nil {
		c.replyError(id, &ResponseError{Code: CodeMethodNotFound, Message: "method not found: " + method})
		return
	}
	res, err := c.handler(context.Background(), method, params)
	if err != nil {
		var re *ResponseError
		if !errors.As(err, &re) {
			re = &ResponseError{Code: CodeInternalError, Message: err.Error()}
		}
		c.replyError(id, re)
		return
	}
	b, err := json.Marshal(res)
	if err != nil {
		c.replyError(id, &ResponseError{Code: CodeInternalError, Message: err.Error()})
		return
	}
	if werr := c.write(response{JSONRPC: "2.0", ID: id, Result: b}); werr != nil {
		c.log.Debug("lsp: reply", "method", method, "err", werr)
	}
}

func (c *Conn) replyError(id json.RawMessage, e *ResponseError) {
	if err := c.write(errorResponse{JSONRPC: "2.0", ID: id, Error: e}); err != nil {
		c.log.Debug("lsp: reply error", "err", err)
	}
}
