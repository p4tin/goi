package dap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

// fakeAdapter reads requests from conn and answers with fn.
type fakeAdapter struct {
	conn net.Conn
	r    *bufio.Reader
	mu   sync.Mutex
	seq  int
}

func (f *fakeAdapter) send(v map[string]any) {
	f.mu.Lock()
	f.seq++
	v["seq"] = f.seq
	f.mu.Unlock()
	b, _ := json.Marshal(v)
	fmt.Fprintf(f.conn, "Content-Length: %d\r\n\r\n%s", len(b), b)
}

func (f *fakeAdapter) next(t *testing.T) message {
	t.Helper()
	body, err := readFrame(f.r)
	if err != nil {
		t.Errorf("adapter read: %v", err) // may run on adapter goroutines
		return message{}
	}
	var m message
	json.Unmarshal(body, &m)
	return m
}

func pair(t *testing.T, onEvent func(Event), reverse ReverseHandler) (*Conn, *fakeAdapter) {
	t.Helper()
	a, b := net.Pipe()
	c := NewConn(a, onEvent, reverse, nil)
	t.Cleanup(func() { c.Close(); b.Close() })
	return c, &fakeAdapter{conn: b, r: bufio.NewReader(b)}
}

func TestCallSuccessAndFailure(t *testing.T) {
	c, f := pair(t, nil, nil)
	go func() {
		m := f.next(t)
		var args SetBreakpointsArguments
		json.Unmarshal(m.Arguments, &args)
		f.send(map[string]any{"type": "response", "request_seq": m.Seq, "command": m.Command, "success": true,
			"body": map[string]any{"breakpoints": []any{map[string]any{"id": 1, "verified": true, "line": args.Breakpoints[0].Line}}}})
		m = f.next(t)
		f.send(map[string]any{"type": "response", "request_seq": m.Seq, "command": m.Command, "success": false, "message": "evaluate",
			"body": map[string]any{"error": map[string]any{"format": "could not find symbol value for x"}}})
	}()
	var out BreakpointsResponse
	err := c.Call(context.Background(), "setBreakpoints", SetBreakpointsArguments{Source: Source{Path: "/a.go"}, Breakpoints: []SourceBreakpoint{{Line: 7, Condition: "i == 2"}}}, &out)
	if err != nil || len(out.Breakpoints) != 1 || out.Breakpoints[0].Line != 7 || !out.Breakpoints[0].Verified {
		t.Fatalf("setBreakpoints = %+v, %v", out, err)
	}
	err = c.Call(context.Background(), "evaluate", map[string]any{"expression": "x"}, nil)
	var de *Error
	if !errors.As(err, &de) || de.Message != "could not find symbol value for x" {
		t.Fatalf("want readable *Error, got %v", err)
	}
}

func TestEventsAreOrderedAndMayCall(t *testing.T) {
	var mu sync.Mutex
	var got []string
	done := make(chan struct{})
	var c *Conn
	c, f := pair(t, func(ev Event) {
		mu.Lock()
		got = append(got, ev.Event)
		n := len(got)
		mu.Unlock()
		if ev.Event == "stopped" {
			// A handler may call back into the connection.
			var th struct {
				Threads []Thread `json:"threads"`
			}
			if err := c.Call(context.Background(), "threads", nil, &th); err != nil || len(th.Threads) != 1 {
				t.Errorf("threads from handler = %+v %v", th, err)
			}
		}
		if n == 3 {
			close(done)
		}
	}, nil)
	go func() {
		f.send(map[string]any{"type": "event", "event": "output", "body": map[string]any{"category": "stdout", "output": "hi\n"}})
		f.send(map[string]any{"type": "event", "event": "stopped", "body": map[string]any{"reason": "breakpoint", "threadId": 1}})
		f.send(map[string]any{"type": "event", "event": "continued"})
		m := f.next(t)
		f.send(map[string]any{"type": "response", "request_seq": m.Seq, "command": "threads", "success": true, "body": map[string]any{"threads": []any{map[string]any{"id": 1, "name": "[Go 1] main.main"}}}})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("events not delivered")
	}
	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(got) != "[output stopped continued]" {
		t.Fatalf("order = %v", got)
	}
}

func TestReverseRequest(t *testing.T) {
	_, f := pair(t, nil, func(cmd string, _ json.RawMessage) (any, error) {
		if cmd == "runInTerminal" {
			return nil, errors.New("not supported")
		}
		return map[string]int{"ok": 1}, nil
	})
	f.send(map[string]any{"type": "request", "command": "runInTerminal", "arguments": map[string]any{}})
	m := f.next(t)
	if m.Type != "response" || m.Success || m.Message != "not supported" {
		t.Fatalf("reverse response = %+v", m)
	}
}

func TestCloseFailsPendingCalls(t *testing.T) {
	c, f := pair(t, nil, nil)
	errc := make(chan error, 1)
	go func() { errc <- c.Call(context.Background(), "threads", nil, nil) }()
	f.next(t)
	f.conn.Close()
	select {
	case err := <-errc:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("want ErrClosed, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pending call not failed")
	}
	if err := c.Call(context.Background(), "threads", nil, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("call after close = %v", err)
	}
}

func TestCallContextCancel(t *testing.T) {
	c, f := pair(t, nil, nil)
	go f.next(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.Call(ctx, "pause", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", err)
	}
}
