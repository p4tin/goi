package gopls

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/lsp"
)

func TestMain(m *testing.M) {
	if os.Getenv("GOI_FAKE_LSP") == "1" {
		fakeServer()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeServer is a scripted LSP server: it echoes configuration and
// document events back as diagnostics and obeys goi/* test requests.
func fakeServer() {
	var conn *lsp.Conn
	publish := func(uri, msg string) {
		conn.Notify(context.Background(), "textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{
			URI: uri, Diagnostics: []lsp.Diagnostic{{Message: msg}},
		})
	}
	progress := func(kind, title string) {
		v, _ := json.Marshal(lsp.WorkDoneProgress{Kind: kind, Title: title})
		conn.Notify(context.Background(), "$/progress", lsp.ProgressParams{Token: "t1", Value: v})
	}
	handler := func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		switch method {
		case "initialize":
			return map[string]any{"capabilities": map[string]any{}}, nil
		case "initialized":
			go func() {
				var cfg []map[string]any
				conn.Call(context.Background(), "workspace/configuration", lsp.ConfigurationParams{Items: []lsp.ConfigurationItem{{Section: "gopls"}}}, &cfg)
				b, _ := json.Marshal(cfg)
				publish("file:///config", string(b))
				if os.Getenv("GOI_FAKE_PROGRESS") == "1" {
					conn.Call(context.Background(), "window/workDoneProgress/create", map[string]any{"token": "t1"}, nil)
					progress("begin", "Loading packages")
				}
			}()
		case "textDocument/didOpen":
			var p struct{ TextDocument lsp.TextDocumentItem }
			json.Unmarshal(params, &p)
			publish(p.TextDocument.URI, fmt.Sprintf("open:%d:%s", p.TextDocument.Version, p.TextDocument.Text))
		case "textDocument/didChange":
			var p struct {
				TextDocument struct {
					URI     string
					Version int
				}
				ContentChanges []struct{ Text string }
			}
			json.Unmarshal(params, &p)
			publish(p.TextDocument.URI, fmt.Sprintf("change:%d:%s", p.TextDocument.Version, p.ContentChanges[0].Text))
		case "textDocument/hover":
			return map[string]any{"contents": map[string]string{"kind": "markdown", "value": "hover"}}, nil
		case "goi/crash":
			os.Exit(3)
		case "goi/endProgress":
			progress("end", "")
		case "goi/applyEdit":
			var res lsp.ApplyWorkspaceEditResult
			err := conn.Call(ctx, "workspace/applyEdit", map[string]any{"label": "fix", "edit": map[string]any{
				"changes": map[string]any{"file:///x/a.go": []lsp.TextEdit{{NewText: "z"}}},
			}}, &res)
			return res, err
		case "shutdown":
			return nil, nil
		case "exit":
			os.Exit(0)
		}
		return nil, nil
	}
	conn = lsp.NewConn(stdio{Reader: os.Stdin, WriteCloser: os.Stdout}, handler, nil)
	<-conn.Done()
}

type recorder struct {
	mu       sync.Mutex
	diags    map[string][]string
	statuses []Status
	edits    []FileEdit
}

func (r *recorder) onDiag(path string, d []lsp.Diagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range d {
		r.diags[path] = append(r.diags[path], x.Message)
	}
}

func (r *recorder) onStatus(s Status) {
	r.mu.Lock()
	r.statuses = append(r.statuses, s)
	r.mu.Unlock()
}

func (r *recorder) has(path, msg string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.diags[path] {
		if strings.Contains(m, msg) {
			return true
		}
	}
	return false
}

func (r *recorder) count(path, msg string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, m := range r.diags[path] {
		if m == msg {
			n++
		}
	}
	return n
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", what)
}

func fakeServerConfig(t *testing.T, r *recorder, extraEnv ...string) Config {
	return Config{
		Bin:           os.Args[0],
		Root:          t.TempDir(),
		Env:           append(append(os.Environ(), "GOI_FAKE_LSP=1"), extraEnv...),
		BuildFlags:    []string{"-tags=integration"},
		Settings:      map[string]any{"usePlaceholders": true},
		OnDiagnostics: r.onDiag,
		OnStatus:      r.onStatus,
		OnApplyEdit: func(_ string, e []FileEdit) bool {
			r.mu.Lock()
			r.edits = append(r.edits, e...)
			r.mu.Unlock()
			return true
		},
		Backoff: func(int) time.Duration { return 10 * time.Millisecond },
	}
}

func startFake(t *testing.T, cfg Config) *Server {
	t.Helper()
	s := New(cfg)
	s.Start()
	t.Cleanup(s.Close)
	return s
}

func TestServerConfigAndDocumentSync(t *testing.T) {
	r := &recorder{diags: map[string][]string{}}
	s := startFake(t, fakeServerConfig(t, r))
	path := filepath.Join(t.TempDir(), "a.go")
	s.Open(path, 1, "a") // before ready: sent when initialized

	eventually(t, "config echoed", func() bool { return r.has(URIToPath("file:///config"), `"buildFlags":["-tags=integration"]`) })
	if !r.has(URIToPath("file:///config"), `"usePlaceholders":true`) {
		t.Errorf("settings missing: %v", r.diags)
	}
	eventually(t, "didOpen", func() bool { return r.has(path, "open:1:a") })
	eventually(t, "ready", func() bool { return s.Status().State == StateReady })

	s.Change(path, 2, "b")
	eventually(t, "didChange", func() bool { return r.has(path, "change:2:b") })
	if v := s.Version(path); v != 2 {
		t.Errorf("version = %d", v)
	}

	h, err := s.Hover(context.Background(), path, lsp.Position{})
	if err != nil || h == nil || h.Contents != "hover" {
		t.Fatalf("hover = %+v, %v", h, err)
	}

	// Re-opening with a restarted (lower) version must reach gopls as a
	// fresh open, not a rejected didChange.
	s.Open(path, 0, "fresh")
	eventually(t, "reopen", func() bool { return r.has(path, "open:0:fresh") })
}

func TestServerRestartsAndReopensAfterCrash(t *testing.T) {
	r := &recorder{diags: map[string][]string{}}
	s := startFake(t, fakeServerConfig(t, r))
	path := filepath.Join(t.TempDir(), "a.go")
	s.Open(path, 1, "a")
	eventually(t, "first open", func() bool { return r.count(path, "open:1:a") == 1 })

	_ = s.call(context.Background(), "goi/crash", nil, nil)
	eventually(t, "reopened after restart", func() bool { return r.count(path, "open:1:a") == 2 })
	eventually(t, "ready again", func() bool { return s.Status().State == StateReady })
	if h, err := s.Hover(context.Background(), path, lsp.Position{}); err != nil || h == nil {
		t.Fatalf("hover after restart: %v", err)
	}
	r.mu.Lock()
	sawError := false
	for _, st := range r.statuses {
		if st.State == StateError && strings.Contains(st.Message, "restarting") {
			sawError = true
		}
	}
	r.mu.Unlock()
	if !sawError {
		t.Errorf("no restarting status: %+v", r.statuses)
	}
}

func TestServerStopsAfterRepeatedCrashes(t *testing.T) {
	r := &recorder{diags: map[string][]string{}}
	cfg := fakeServerConfig(t, r)
	cfg.MaxCrashes = 2
	s := startFake(t, cfg)
	for range 2 {
		eventually(t, "ready", func() bool { return s.Status().State == StateReady })
		_ = s.call(context.Background(), "goi/crash", nil, nil)
		eventually(t, "crash noticed", func() bool { return s.Status().State != StateReady })
	}
	eventually(t, "gave up", func() bool {
		st := s.Status()
		return st.State == StateError && strings.Contains(st.Message, "repeated")
	})
	if _, err := s.Hover(context.Background(), "x.go", lsp.Position{}); !errors.Is(err, ErrNotReady) {
		t.Errorf("hover after give-up: %v", err)
	}
	s.Restart()
	eventually(t, "ready after manual restart", func() bool { return s.Status().State == StateReady })
}

func TestServerProgressStatus(t *testing.T) {
	r := &recorder{diags: map[string][]string{}}
	s := startFake(t, fakeServerConfig(t, r, "GOI_FAKE_PROGRESS=1"))
	eventually(t, "indexing", func() bool {
		st := s.Status()
		return st.State == StateIndexing && strings.Contains(st.Message, "Loading packages")
	})
	if err := s.call(context.Background(), "goi/endProgress", nil, nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "ready", func() bool { return s.Status().State == StateReady })
}

func TestServerApplyEditCallback(t *testing.T) {
	r := &recorder{diags: map[string][]string{}}
	s := startFake(t, fakeServerConfig(t, r))
	var res lsp.ApplyWorkspaceEditResult
	if err := s.call(context.Background(), "goi/applyEdit", nil, &res); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !res.Applied || len(r.edits) != 1 || r.edits[0].Path != URIToPath("file:///x/a.go") || r.edits[0].Edits[0].NewText != "z" {
		t.Fatalf("res=%+v edits=%+v", res, r.edits)
	}
}

func TestServerClose(t *testing.T) {
	r := &recorder{diags: map[string][]string{}}
	s := startFake(t, fakeServerConfig(t, r))
	eventually(t, "ready", func() bool { return s.Status().State == StateReady })
	s.Close()
	if st := s.Status(); st.State != StateStopped {
		t.Errorf("status = %+v", st)
	}
	if _, err := s.Hover(context.Background(), "x.go", lsp.Position{}); !errors.Is(err, ErrNotReady) {
		t.Errorf("after close: %v", err)
	}
}

func TestLanguageID(t *testing.T) {
	for in, want := range map[string]string{"a.go": "go", "/x/go.mod": "go.mod", "go.work": "go.work", "go.sum": "go.sum", "a.txt": ""} {
		if got := LanguageID(in); got != want {
			t.Errorf("%s: %q", in, got)
		}
	}
}

// TestRealGopls exercises the real gopls binary when it is installed.
func TestRealGopls(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go")
	}
	out, _ := exec.Command(goBin, "env", "GOBIN", "GOPATH").Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	opts := ResolveOptions{}
	if len(lines) == 2 {
		opts.GOBIN, opts.GOPATH = lines[0], lines[1]
	}
	bin, err := Resolve(opts)
	if err != nil {
		t.Skip("gopls not installed")
	}
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.22\n"), 0o644)
	src := "package main\n\nimport \"fmt\"\n\nfunc helper() int { return 1 }\n\nfunc main() {\n\t_ = helper()\n\tfmt.Println()\n}\n"
	main := filepath.Join(root, "main.go")
	os.WriteFile(main, []byte(src), 0o644)

	r := &recorder{diags: map[string][]string{}}
	s := New(Config{Bin: bin, Root: root, Env: os.Environ(), OnDiagnostics: r.onDiag, OnStatus: r.onStatus})
	s.Start()
	defer s.Close()
	s.Open(main, 1, src)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	locs, err := s.Definition(ctx, main, lsp.Position{Line: 7, Character: 6})
	if err != nil || len(locs) != 1 || locs[0].Path != main || locs[0].Range.Start.Line != 4 {
		t.Fatalf("definition = %+v, %v", locs, err)
	}

	edited := strings.Replace(src, "\tfmt.Println()\n", "\tfmt.Pr\n", 1)
	s.Change(main, 2, edited)
	list, err := s.Completion(ctx, main, lsp.Position{Line: 8, Character: 7})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range list.Items {
		if it.Label == "Println" || it.Label == "Printf" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no Print* completion in %d items", len(list.Items))
	}
	eventually(t, "diagnostics for broken code", func() bool { return r.has(main, "") })
}

// TestCompletionLatency measures completion on this repository with the real
// gopls (R12.2). Opt-in: GOI_GOPLS_BENCH=1 go test -run CompletionLatency -v.
func TestCompletionLatency(t *testing.T) {
	if os.Getenv("GOI_GOPLS_BENCH") != "1" {
		t.Skip("set GOI_GOPLS_BENCH=1")
	}
	out, _ := exec.Command("go", "env", "GOBIN", "GOPATH").Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	opts := ResolveOptions{}
	if len(lines) == 2 {
		opts.GOBIN, opts.GOPATH = lines[0], lines[1]
	}
	bin, err := Resolve(opts)
	if err != nil {
		t.Skip("gopls not installed")
	}
	root, _ := filepath.Abs("../..")
	file := filepath.Join(root, "internal", "rpc", "lang.go")
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	marker := "ls.app.bus.Emit(events.TopicLangDiagnostics"
	off := strings.Index(text, marker) + len("ls.app.")
	line := strings.Count(text[:off], "\n")
	char := off - (strings.LastIndex(text[:off], "\n") + 1)

	s := New(Config{Bin: bin, Root: root, Env: os.Environ()})
	s.Start()
	defer s.Close()
	s.Open(file, 1, text)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := s.Completion(ctx, file, lsp.Position{Line: line, Character: char}); err != nil { // warm-up
		t.Fatal(err)
	}
	var times []time.Duration
	for range 20 {
		start := time.Now()
		list, err := s.Completion(ctx, file, lsp.Position{Line: line, Character: char})
		if err != nil || len(list.Items) == 0 {
			t.Fatalf("completion: %d items, %v", len(list.Items), err)
		}
		times = append(times, time.Since(start))
	}
	slices.Sort(times)
	t.Logf("completion on %s: p50=%s p95=%s", filepath.Base(root), times[10], times[18])
	if times[18] > 150*time.Millisecond {
		t.Errorf("p95 %s exceeds 150ms", times[18])
	}
}
