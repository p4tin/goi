package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	s := NewServer(Options{
		Token: "secret",
		Info:  Implementation{Name: "goi", Version: "test"},
		Tools: []Tool{
			{Name: "echo", Description: "Echo", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "fail", Description: "Fail", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		Call: func(_ context.Context, target, name string, args json.RawMessage) (ToolResult, error) {
			calls = append(calls, target+":"+name+":"+string(args))
			if name == "fail" {
				return ToolResult{}, errors.New("it broke")
			}
			return TextResult("echo " + string(args)), nil
		},
		Target: func(r *http.Request) string { return strings.TrimPrefix(r.URL.Path, "/mcp/") },
	})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return ts, &calls
}

func rpc(t *testing.T, url, token, body string, hdr map[string]string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestAuthOriginAndMethod(t *testing.T) {
	ts, _ := testServer(t)
	ping := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	if code, _ := rpc(t, ts.URL+"/mcp/w1", "", ping, nil); code != http.StatusUnauthorized {
		t.Errorf("no token: %d", code)
	}
	if code, _ := rpc(t, ts.URL+"/mcp/w1", "wrong", ping, nil); code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", code)
	}
	if code, _ := rpc(t, ts.URL+"/mcp/w1", "secret", ping, map[string]string{"Origin": "https://evil.example"}); code != http.StatusForbidden {
		t.Errorf("foreign origin: %d", code)
	}
	if code, body := rpc(t, ts.URL+"/mcp/w1", "secret", ping, map[string]string{"Origin": "http://localhost:3000"}); code != 200 || !strings.Contains(body, `"result":{}`) {
		t.Errorf("ping: %d %s", code, body)
	}
	resp, _ := http.Get(ts.URL + "/mcp/w1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", resp.StatusCode)
	}
}

func TestInitializeListCall(t *testing.T) {
	ts, calls := testServer(t)
	u := ts.URL + "/mcp/w7/s2"
	_, body := rpc(t, u, "secret", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`, nil)
	if !strings.Contains(body, `"protocolVersion":"2025-03-26"`) || !strings.Contains(body, `"name":"goi"`) {
		t.Errorf("initialize = %s", body)
	}
	_, body = rpc(t, u, "secret", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`, nil)
	if !strings.Contains(body, `"protocolVersion":"`+Versions[0]+`"`) {
		t.Errorf("unknown version answered with %s", body)
	}
	if code, body := rpc(t, u, "secret", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil); code != http.StatusAccepted || body != "" {
		t.Errorf("notification: %d %q", code, body)
	}
	_, body = rpc(t, u, "secret", `{"jsonrpc":"2.0","id":"a","method":"tools/list"}`, nil)
	if !strings.Contains(body, `"name":"echo"`) || !strings.Contains(body, `"id":"a"`) {
		t.Errorf("tools/list = %s", body)
	}
	_, body = rpc(t, u, "secret", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`, nil)
	if !strings.Contains(body, `echo {\"x\":1}`) {
		t.Errorf("tools/call = %s", body)
	}
	_, body = rpc(t, u, "secret", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"fail"}}`, nil)
	if !strings.Contains(body, `"isError":true`) || !strings.Contains(body, "it broke") {
		t.Errorf("failing tool = %s", body)
	}
	_, body = rpc(t, u, "secret", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope"}}`, nil)
	if !strings.Contains(body, `-32602`) {
		t.Errorf("unknown tool = %s", body)
	}
	_, body = rpc(t, u, "secret", `{"jsonrpc":"2.0","id":6,"method":"resources/list"}`, nil)
	if !strings.Contains(body, `-32601`) {
		t.Errorf("unknown method = %s", body)
	}
	if want := []string{`w7/s2:echo:{"x":1}`, `w7/s2:fail:{}`}; strings.Join(*calls, ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v", *calls)
	}
}

func TestBridge(t *testing.T) {
	ts, _ := testServer(t)
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{}}}`)
	var out bytes.Buffer
	if err := Bridge(context.Background(), in, &out, ts.URL+"/mcp/x", "secret", nil); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"id":1`) || !strings.Contains(lines[1], "echo {}") {
		t.Errorf("bridge output:\n%s", out.String())
	}

	// A transport failure becomes a JSON-RPC error for that request.
	out.Reset()
	in = strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"ping"}` + "\n")
	if err := Bridge(context.Background(), in, &out, ts.URL+"/mcp/x", "bad", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":9`) || !strings.Contains(out.String(), "HTTP 401") {
		t.Errorf("error output = %s", out.String())
	}
}
