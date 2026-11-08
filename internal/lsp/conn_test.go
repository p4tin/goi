package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"
)

// pair returns a client and a "server" connected by an in-memory pipe.
func pair(t *testing.T, client, server Handler) (*Conn, *Conn) {
	t.Helper()
	a, b := net.Pipe()
	c := NewConn(a, client, nil)
	s := NewConn(b, server, nil)
	t.Cleanup(func() {
		c.Close()
		s.Close()
	})
	return c, s
}

func TestCallRoundTrip(t *testing.T) {
	c, _ := pair(t, nil, func(_ context.Context, method string, params json.RawMessage) (any, error) {
		if method != "echo" {
			return nil, &ResponseError{Code: CodeMethodNotFound, Message: method}
		}
		var p map[string]int
		json.Unmarshal(params, &p)
		return map[string]int{"n": p["n"] * 2}, nil
	})
	var out map[string]int
	if err := c.Call(context.Background(), "echo", map[string]int{"n": 21}, &out); err != nil {
		t.Fatal(err)
	}
	if out["n"] != 42 {
		t.Fatalf("got %v", out)
	}
}

func TestCallNullResult(t *testing.T) {
	c, _ := pair(t, nil, func(context.Context, string, json.RawMessage) (any, error) { return nil, nil })
	if err := c.Call(context.Background(), "x", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCallError(t *testing.T) {
	c, _ := pair(t, nil, func(context.Context, string, json.RawMessage) (any, error) {
		return nil, &ResponseError{Code: 7, Message: "nope"}
	})
	err := c.Call(context.Background(), "x", nil, nil)
	var re *ResponseError
	if !errors.As(err, &re) || re.Code != 7 || re.Message != "nope" {
		t.Fatalf("err = %v", err)
	}
}

func TestPlainErrorBecomesInternalError(t *testing.T) {
	c, _ := pair(t, nil, func(context.Context, string, json.RawMessage) (any, error) {
		return nil, errors.New("boom")
	})
	var re *ResponseError
	if err := c.Call(context.Background(), "x", nil, nil); !errors.As(err, &re) || re.Code != CodeInternalError {
		t.Fatalf("err = %v", err)
	}
}

func TestServerRequestAndNotification(t *testing.T) {
	got := make(chan string, 2)
	_, s := pair(t, func(_ context.Context, method string, params json.RawMessage) (any, error) {
		got <- method
		if method == "workspace/configuration" {
			return []any{map[string]bool{"ok": true}}, nil
		}
		return nil, nil
	}, nil)
	var cfg []map[string]bool
	if err := s.Call(context.Background(), "workspace/configuration", ConfigurationParams{}, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg) != 1 || !cfg[0]["ok"] {
		t.Fatalf("cfg = %v", cfg)
	}
	if err := s.Notify(context.Background(), "window/logMessage", MessageParams{Type: 3, Message: "hi"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"workspace/configuration", "window/logMessage"} {
		select {
		case m := <-got:
			if m != want {
				t.Errorf("got %q, want %q", m, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for %s", want)
		}
	}
}

func TestNilHandlerRepliesMethodNotFound(t *testing.T) {
	_, s := pair(t, nil, nil)
	var re *ResponseError
	if err := s.Call(context.Background(), "x", nil, nil); !errors.As(err, &re) || re.Code != CodeMethodNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestCancelSendsCancelRequest(t *testing.T) {
	cancelled := make(chan any, 1)
	release := make(chan struct{})
	c, _ := pair(t, nil, func(_ context.Context, method string, params json.RawMessage) (any, error) {
		switch method {
		case "slow":
			<-release
			return 1, nil
		case "$/cancelRequest":
			var p struct{ ID any }
			json.Unmarshal(params, &p)
			cancelled <- p.ID
		}
		return nil, nil
	})
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.Call(ctx, "slow", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	select {
	case id := <-cancelled:
		if id == nil {
			t.Error("cancel without id")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no $/cancelRequest")
	}
}

func TestCloseFailsPendingCalls(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	c, s := pair(t, nil, func(context.Context, string, json.RawMessage) (any, error) {
		<-block
		return nil, nil
	})
	errc := make(chan error, 1)
	go func() { errc <- c.Call(context.Background(), "hang", nil, nil) }()
	time.Sleep(20 * time.Millisecond)
	s.Close()
	select {
	case err := <-errc:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending call not failed")
	}
	if err := c.Call(context.Background(), "after", nil, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("after close: %v", err)
	}
}
