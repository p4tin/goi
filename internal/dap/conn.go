// Package dap is a minimal Debug Adapter Protocol client: Content-Length
// framed JSON messages over a stream, sequenced requests, ordered events,
// and protocol types for the subset Goi uses. It has no Goi knowledge.
package dap

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
	"time"
)

// ErrClosed is returned by calls made after the connection closed.
var ErrClosed = errors.New("dap: connection closed")

// Error is a failed response from the adapter.
type Error struct {
	Command string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("dap: %s: %s", e.Command, e.Message) }

// Event is an adapter event.
type Event struct {
	Event string          `json:"event"`
	Body  json.RawMessage `json:"body,omitempty"`
}

// ReverseHandler answers requests sent by the adapter (such as
// runInTerminal). Returning an error sends a failed response.
type ReverseHandler func(command string, args json.RawMessage) (any, error)

type message struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	Command    string          `json:"command,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	RequestSeq int             `json:"request_seq,omitempty"`
	Success    bool            `json:"success,omitempty"`
	Message    string          `json:"message,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
	Event      string          `json:"event,omitempty"`
}

type request struct {
	Seq       int    `json:"seq"`
	Type      string `json:"type"`
	Command   string `json:"command"`
	Arguments any    `json:"arguments,omitempty"`
}

type response struct {
	Seq        int    `json:"seq"`
	Type       string `json:"type"`
	RequestSeq int    `json:"request_seq"`
	Command    string `json:"command"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	Body       any    `json:"body,omitempty"`
}

type result struct {
	msg message
	err error
}

// Conn is a DAP client connection.
type Conn struct {
	rwc     io.ReadWriteCloser
	onEvent func(Event)
	reverse ReverseHandler
	log     *slog.Logger

	writeMu sync.Mutex

	mu      sync.Mutex
	seq     int
	pending map[int]chan result
	closed  bool

	evMu       sync.Mutex
	evCond     *sync.Cond
	evQ        []Event
	evDone     bool
	delivering bool

	done chan struct{}
}

// NewConn starts reading from rwc. onEvent receives events one at a time,
// in order, on a dedicated goroutine (it may make calls on the Conn).
func NewConn(rwc io.ReadWriteCloser, onEvent func(Event), reverse ReverseHandler, log *slog.Logger) *Conn {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	c := &Conn{rwc: rwc, onEvent: onEvent, reverse: reverse, log: log, pending: map[int]chan result{}, done: make(chan struct{})}
	c.evCond = sync.NewCond(&c.evMu)
	go c.read()
	go c.deliver()
	return c
}

// Done is closed when the connection stops reading.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Close closes the stream and fails pending calls.
func (c *Conn) Close() error {
	err := c.rwc.Close()
	<-c.done
	return err
}

// Call sends a request and decodes the response body into out (may be nil).
// A failed response becomes *Error. If ctx ends first, ctx.Err() is
// returned and the late response is dropped.
func (c *Conn) Call(ctx context.Context, command string, args, out any) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.seq++
	seq := c.seq
	ch := make(chan result, 1)
	c.pending[seq] = ch
	c.mu.Unlock()

	if err := c.write(request{Seq: seq, Type: "request", Command: command, Arguments: args}); err != nil {
		c.forget(seq)
		return err
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if !r.msg.Success {
			msg := r.msg.Message
			// Adapters put the readable text in body.error.format.
			var b struct {
				Error *struct {
					Format string `json:"format"`
				} `json:"error"`
			}
			if json.Unmarshal(r.msg.Body, &b) == nil && b.Error != nil && b.Error.Format != "" {
				msg = b.Error.Format
			}
			return &Error{Command: command, Message: msg}
		}
		if out == nil || len(r.msg.Body) == 0 {
			return nil
		}
		if err := json.Unmarshal(r.msg.Body, out); err != nil {
			return fmt.Errorf("dap: decode %s response: %w", command, err)
		}
		return nil
	case <-ctx.Done():
		c.forget(seq)
		return ctx.Err()
	}
}

func (c *Conn) forget(seq int) {
	c.mu.Lock()
	delete(c.pending, seq)
	c.mu.Unlock()
}

func (c *Conn) nextSeq() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	return c.seq
}

func (c *Conn) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("dap: encode: %w", err)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := fmt.Fprintf(c.rwc, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return fmt.Errorf("dap: write: %w", err)
	}
	if _, err := c.rwc.Write(b); err != nil {
		return fmt.Errorf("dap: write: %w", err)
	}
	return nil
}

func (c *Conn) read() {
	r := bufio.NewReaderSize(c.rwc, 64*1024)
	for {
		body, err := readFrame(r)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.log.Debug("dap: read", "err", err)
			}
			break
		}
		var m message
		if err := json.Unmarshal(body, &m); err != nil {
			c.log.Warn("dap: bad message", "err", err)
			continue
		}
		switch m.Type {
		case "response":
			c.mu.Lock()
			ch := c.pending[m.RequestSeq]
			delete(c.pending, m.RequestSeq)
			c.mu.Unlock()
			if ch != nil {
				ch <- result{msg: m}
			}
		case "event":
			c.evMu.Lock()
			c.evQ = append(c.evQ, Event{Event: m.Event, Body: m.Body})
			c.evCond.Signal()
			c.evMu.Unlock()
		case "request":
			go c.answer(m)
		}
	}
	c.mu.Lock()
	c.closed = true
	pending := c.pending
	c.pending = map[int]chan result{}
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- result{err: ErrClosed}
	}
	c.evMu.Lock()
	c.evDone = true
	c.evCond.Signal()
	c.evMu.Unlock()
	close(c.done)
}

// deliver hands queued events to onEvent in order.
func (c *Conn) deliver() {
	for {
		c.evMu.Lock()
		for len(c.evQ) == 0 && !c.evDone {
			c.evCond.Wait()
		}
		if len(c.evQ) == 0 && c.evDone {
			c.evMu.Unlock()
			return
		}
		ev := c.evQ[0]
		c.evQ = c.evQ[1:]
		c.delivering = true
		c.evMu.Unlock()
		if c.onEvent != nil {
			c.onEvent(ev)
		}
		c.evMu.Lock()
		c.delivering = false
		c.evCond.Broadcast()
		c.evMu.Unlock()
	}
}

// DrainEvents waits (up to timeout) until every event received so far has
// been delivered. Call it after a response to see the events the adapter
// sent before it. It must not be called from the event handler.
func (c *Conn) DrainEvents(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		c.evMu.Lock()
		idle := len(c.evQ) == 0 && !c.delivering
		c.evMu.Unlock()
		if idle || time.Now().After(deadline) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (c *Conn) answer(m message) {
	resp := response{Seq: c.nextSeq(), Type: "response", RequestSeq: m.Seq, Command: m.Command}
	if c.reverse == nil {
		resp.Message = "not supported: " + m.Command
	} else if body, err := c.reverse(m.Command, m.Arguments); err != nil {
		resp.Message = err.Error()
	} else {
		resp.Success, resp.Body = true, body
	}
	if err := c.write(resp); err != nil {
		c.log.Debug("dap: reverse response", "command", m.Command, "err", err)
	}
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
			if length < 0 {
				continue // tolerate stray blank lines between frames
			}
			break
		}
		name, val, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("dap: bad Content-Length %q", val)
			}
			length = n
		}
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
