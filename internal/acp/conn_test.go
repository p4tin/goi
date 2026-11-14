package acp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// pair connects two Conns back to back.
func pair(t *testing.T, ha, hb Handler) (a, b *Conn) {
	t.Helper()
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	a = NewConn(ar, aw, closers{ar, aw}, ha, nil)
	b = NewConn(br, bw, closers{br, bw}, hb, nil)
	t.Cleanup(func() {
		a.Close()
		b.Close()
	})
	return a, b
}

type closers []io.Closer

func (cs closers) Close() error {
	for _, c := range cs {
		c.Close()
	}
	return nil
}

func TestCallAndNestedRequest(t *testing.T) {
	var b *Conn
	// The client side serves fs/read_text_file.
	ha := func(_ context.Context, method string, p json.RawMessage) (any, error) {
		var rp ReadTextFileParams
		json.Unmarshal(p, &rp)
		return ReadTextFileResult{Content: "read " + rp.Path}, nil
	}
	// The agent side answers "prompt" only after calling back into the
	// client, like an agent reading a file in the middle of a turn.
	hb := func(ctx context.Context, method string, _ json.RawMessage) (any, error) {
		var got ReadTextFileResult
		if err := b.Call(ctx, MethodReadTextFile, ReadTextFileParams{Path: "/x"}, &got); err != nil {
			return nil, err
		}
		return PromptResult{StopReason: got.Content}, nil
	}
	a, bb := pair(t, ha, hb)
	b = bb
	var res PromptResult
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Call(ctx, "prompt", nil, &res); err != nil {
		t.Fatal(err)
	}
	if res.StopReason != "read /x" {
		t.Errorf("result = %q", res.StopReason)
	}
}

func TestNotificationsStayOrdered(t *testing.T) {
	var mu sync.Mutex
	var got []string
	done := make(chan struct{})
	hb := func(_ context.Context, method string, p json.RawMessage) (any, error) {
		var n SessionNotification
		json.Unmarshal(p, &n)
		mu.Lock()
		got = append(got, string(n.Update))
		if len(got) == 100 {
			close(done)
		}
		mu.Unlock()
		return nil, nil
	}
	a, _ := pair(t, nil, hb)
	for i := range 100 {
		if err := a.Notify(MethodSessionUpdate, SessionNotification{SessionID: "s", Update: json.RawMessage(itoa(i))}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("notifications not delivered")
	}
	for i, s := range got {
		if s != itoa(i) {
			t.Fatalf("notification %d = %s", i, s)
		}
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func TestErrors(t *testing.T) {
	hb := func(_ context.Context, method string, _ json.RawMessage) (any, error) {
		switch method {
		case "auth":
			return nil, &Error{Code: CodeAuthRequired, Message: "Authentication required"}
		case "boom":
			return nil, errors.New("boom")
		}
		return nil, nil
	}
	a, _ := pair(t, nil, hb)
	ctx := context.Background()
	if err := a.Call(ctx, "auth", nil, nil); !IsAuthRequired(err) {
		t.Errorf("auth err = %v", err)
	}
	var e *Error
	if err := a.Call(ctx, "boom", nil, nil); !errors.As(err, &e) || e.Code != CodeInternalError || e.Message != "boom" {
		t.Errorf("boom err = %v", err)
	}
	if err := a.Call(ctx, "ok", nil, nil); err != nil {
		t.Errorf("ok err = %v", err)
	}
}

func TestNilHandlerAndBadLines(t *testing.T) {
	r, w := io.Pipe()
	out := &strings.Builder{}
	var mu sync.Mutex
	c := NewConn(r, writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return out.Write(p)
	}), r, nil, nil)
	io.WriteString(w, "not json\n\n")
	io.WriteString(w, `{"jsonrpc":"2.0","id":"x","method":"fs/read_text_file","params":{}}`+"\n")
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		s := out.String()
		mu.Unlock()
		if strings.Contains(s, `"id":"x"`) {
			if !strings.Contains(s, `-32601`) {
				t.Errorf("reply = %s", s)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no reply")
		}
		time.Sleep(10 * time.Millisecond)
	}
	w.Close()
	c.Close()
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestCloseFailsPendingCalls(t *testing.T) {
	block := make(chan struct{})
	hb := func(context.Context, string, json.RawMessage) (any, error) {
		<-block
		return nil, nil
	}
	a, b := pair(t, nil, hb)
	errc := make(chan error, 1)
	go func() { errc <- a.Call(context.Background(), "slow", nil, nil) }()
	time.Sleep(50 * time.Millisecond)
	b.Close()
	a.Close()
	select {
	case err := <-errc:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pending call not failed")
	}
	close(block)
	if err := a.Call(context.Background(), "x", nil, nil); !errors.Is(err, ErrClosed) {
		t.Errorf("call after close = %v", err)
	}
}

func TestMCPServerJSON(t *testing.T) {
	b, _ := json.Marshal([]MCPServer{
		{Name: "goi", URL: "http://127.0.0.1:1/mcp", Headers: []NameValue{{"Authorization", "Bearer t"}}},
		{Name: "goi", Command: "/bin/goi", Args: []string{"mcp-bridge"}},
	})
	want := `[{"type":"http","name":"goi","url":"http://127.0.0.1:1/mcp","headers":[{"name":"Authorization","value":"Bearer t"}]},{"name":"goi","command":"/bin/goi","args":["mcp-bridge"],"env":[]}]`
	if string(b) != want {
		t.Errorf("json =\n%s\nwant\n%s", b, want)
	}
	var back []MCPServer
	if err := json.Unmarshal(b, &back); err != nil || back[0].URL == "" || back[1].Command != "/bin/goi" {
		t.Errorf("round trip = %+v, %v", back, err)
	}
}
