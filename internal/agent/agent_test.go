package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/acp"
	"github.com/p4tin/goi/internal/acp/acptest"
)

func TestMain(m *testing.M) {
	if os.Getenv("GOI_FAKE_AGENT") == "1" {
		acptest.Main()
	}
	os.Exit(m.Run())
}

func TestSplitCommandLine(t *testing.T) {
	cases := map[string][]string{
		`npx -y pkg`:                 {"npx", "-y", "pkg"},
		`  a   'b c'  "d \"e\"" `:    {"a", "b c", `d "e"`},
		`x ''`:                       {"x", ""},
		`gemini --acp --model="g 2"`: {"gemini", "--acp", "--model=g 2"},
	}
	if runtime.GOOS != "windows" {
		cases[`a\ b c`] = []string{"a b", "c"}
	}
	for in, want := range cases {
		got, err := SplitCommandLine(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%q → %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", `a 'b`, `a "b`} {
		if _, err := SplitCommandLine(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
	if got := JoinCommandLine([]string{"claude", "--login", "a b"}); runtime.GOOS != "windows" && got != `claude --login 'a b'` {
		t.Errorf("join = %s", got)
	}
}

func TestResolve(t *testing.T) {
	tools := t.TempDir()
	def, _ := Lookup("gemini")
	noPath := func(string) (string, error) { return "", errors.New("not found") }
	onPath := func(f string) (string, error) { return "/usr/bin/" + f, nil }

	if _, err := Resolve(def, "", tools, noPath); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("nothing installed: %v", err)
	}
	c, err := Resolve(def, "", tools, onPath)
	if err != nil || c.Path != "/usr/bin/gemini" || c.Source != SourcePath || !reflect.DeepEqual(c.Args, []string{"--acp"}) {
		t.Errorf("PATH: %+v %v", c, err)
	}
	bin := filepath.Join(InstallDir(tools, "gemini"), "node_modules", ".bin", binName("gemini"))
	os.MkdirAll(filepath.Dir(bin), 0o755)
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	if c, err := Resolve(def, "", tools, onPath); err != nil || c.Path != bin || c.Source != SourceTools {
		t.Errorf("tools dir: %+v %v", c, err)
	}
	if c, err := Resolve(def, `/opt/g/gemini --acp --debug`, tools, onPath); err != nil || c.Path != "/opt/g/gemini" || c.Source != SourceSetting || len(c.Args) != 2 {
		t.Errorf("override: %+v %v", c, err)
	}
	if c, err := Resolve(Definition{ID: "mine"}, `myagent serve`, "", onPath); err != nil || c.Path != "/usr/bin/myagent" || c.Args[0] != "serve" {
		t.Errorf("custom: %+v %v", c, err)
	}
	if _, err := Resolve(Definition{ID: "mine"}, "", "", onPath); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("custom without command: %v", err)
	}
	defs := Definitions(map[string]string{"zeta": "z", "alpha": "a", "claude": "x", "empty": " "})
	var ids []string
	for _, d := range defs {
		ids = append(ids, d.ID)
	}
	if strings.Join(ids, ",") != "claude,gemini,codex,alpha,zeta" {
		t.Errorf("definitions = %v", ids)
	}
}

func TestInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as npm")
	}
	dir := t.TempDir()
	npm := filepath.Join(dir, "npm")
	os.WriteFile(npm, []byte("#!/bin/sh\necho \"args: $*\"\necho warn >&2\n[ \"$FAIL\" = 1 ] && exit 3\nexit 0\n"), 0o755)
	def, _ := Lookup("claude")
	var lines []string
	if err := Install(context.Background(), def, dir, npm, os.Environ(), func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || !strings.Contains(lines[0], "install --prefix "+InstallDir(dir, "claude")) || !strings.Contains(lines[0], def.Package) {
		t.Errorf("lines = %q", lines)
	}
	err := Install(context.Background(), def, dir, npm, append(os.Environ(), "FAIL=1"), nil)
	if err == nil || !strings.Contains(err.Error(), "warn") {
		t.Errorf("failing install: %v", err)
	}
	if _, err := FindNpm(func(string) (string, error) { return "", errors.New("no") }); !errors.Is(err, ErrNoNpm) {
		t.Errorf("FindNpm: %v", err)
	}
}

func TestTerminals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	var mu sync.Mutex
	var chunks []string
	var exits []*acp.ExitStatus
	ts := NewTerminals("a1-", func(id string, data []byte, exit *acp.ExitStatus) {
		mu.Lock()
		defer mu.Unlock()
		if exit != nil {
			exits = append(exits, exit)
		} else {
			chunks = append(chunks, string(data))
		}
	})
	id, err := ts.Create(TerminalSpec{Command: "printf 'héllo wörld'; exit 3", Dir: t.TempDir(), Limit: 6})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "a1-t") {
		t.Errorf("id = %s", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := ts.Wait(ctx, id)
	if err != nil || st.ExitCode == nil || *st.ExitCode != 3 {
		t.Fatalf("wait = %+v %v", st, err)
	}
	out, _ := ts.Output(id)
	if !out.Truncated || out.Output != "wörld" || out.ExitStatus == nil {
		t.Errorf("output = %+v", out)
	}
	mu.Lock()
	if strings.Join(chunks, "") != "héllo wörld" || len(exits) != 1 {
		t.Errorf("chunks %q exits %d", chunks, len(exits))
	}
	mu.Unlock()

	long, _ := ts.Create(TerminalSpec{Command: "sleep", Args: []string{"30"}})
	if err := ts.Kill(long); err != nil {
		t.Fatal(err)
	}
	if st, err := ts.Wait(ctx, long); err != nil || st.Signal == "" && (st.ExitCode == nil || *st.ExitCode == 0) {
		t.Errorf("killed = %+v %v", st, err)
	}
	if err := ts.Release(long); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Output(long); !errors.Is(err, ErrNoTerminal) {
		t.Errorf("after release: %v", err)
	}
}

// fakeHost serves files from memory and answers permissions with allow.
type fakeHost struct {
	mu      sync.Mutex
	files   map[string]string
	updates []string
	perm    func(acp.RequestPermissionParams) (acp.PermissionOutcome, error)
	dir     string
	termOut strings.Builder
}

func (h *fakeHost) SessionUpdate(_ string, u json.RawMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.updates = append(h.updates, string(u))
}

func (h *fakeHost) RequestPermission(ctx context.Context, p acp.RequestPermissionParams) (acp.PermissionOutcome, error) {
	if h.perm != nil {
		return h.perm(p)
	}
	return acp.PermissionOutcome{Outcome: "selected", OptionID: p.Options[0].OptionID}, nil
}

func (h *fakeHost) ReadTextFile(_ context.Context, p acp.ReadTextFileParams) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.files[p.Path]
	if !ok {
		return "", errors.New("no such file")
	}
	return s, nil
}

func (h *fakeHost) WriteTextFile(_ context.Context, p acp.WriteTextFileParams) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.files[p.Path] = p.Content
	return nil
}

func (h *fakeHost) TerminalSpec(_, cwd string) (string, []string, error) {
	return h.dir, os.Environ(), nil
}

func (h *fakeHost) TerminalOutput(_ string, data []byte, _ *acp.ExitStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.termOut.Write(data)
}

func (h *fakeHost) text() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var b strings.Builder
	for _, u := range h.updates {
		var v struct {
			Kind    string `json:"sessionUpdate"`
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		json.Unmarshal([]byte(u), &v)
		if v.Kind == "agent_message_chunk" {
			b.WriteString(v.Content.Text)
		}
	}
	h.updates = nil
	return b.String()
}

func startFake(t *testing.T, h *fakeHost, env ...string) *Client {
	t.Helper()
	exe, _ := os.Executable()
	c, err := Start(Options{
		Command: Command{Path: exe, Args: []string{"-test.run=^$"}},
		Dir:     t.TempDir(),
		Env:     append(append(os.Environ(), "GOI_FAKE_AGENT=1"), env...),
		Host:    h,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestClientSession(t *testing.T) {
	h := &fakeHost{files: map[string]string{"/p/a.go": "package a"}, dir: t.TempDir()}
	c := startFake(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := c.Initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !info.AgentCapabilities.MCPCapabilities.HTTP || len(info.AuthMethods) != 2 || c.Info().AgentInfo.Name != "fake-agent" {
		t.Errorf("info = %+v", info)
	}
	ns, err := c.NewSession(ctx, "/p", nil)
	if err != nil || ns.SessionID == "" || ns.Modes == nil || ns.Modes.CurrentModeID != "ask" {
		t.Fatalf("new session = %+v %v", ns, err)
	}
	sid := ns.SessionID

	turn := func(text string) (string, string) {
		t.Helper()
		stop, err := c.Prompt(ctx, sid, []acp.ContentBlock{acp.TextBlock(text)})
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		return stop, h.text()
	}
	if stop, out := turn("echo hello there"); stop != acp.StopEndTurn || out != "hello there" {
		t.Errorf("echo: %s %q", stop, out)
	}
	if _, out := turn("read /p/a.go"); out != "content: package a" {
		t.Errorf("read: %q", out)
	}
	if _, out := turn("write /p/b.go package b"); out != "wrote /p/b.go" || h.files["/p/b.go"] != "package b" {
		t.Errorf("write: %q files %v", out, h.files)
	}
	h.perm = func(acp.RequestPermissionParams) (acp.PermissionOutcome, error) {
		return acp.PermissionOutcome{Outcome: "selected", OptionID: "reject"}, nil
	}
	if _, out := turn("write /p/c.go package c"); out != "rejected" || h.files["/p/c.go"] != "" {
		t.Errorf("rejected write: %q", out)
	}
	if runtime.GOOS != "windows" {
		if _, out := turn("run echo from-terminal"); out != "exit 0: from-terminal" {
			t.Errorf("run: %q", out)
		}
		if !strings.Contains(h.termOut.String(), "from-terminal") {
			t.Errorf("terminal output not streamed: %q", h.termOut.String())
		}
	}
	if err := c.SetMode(ctx, sid, "code"); err != nil {
		t.Errorf("set mode: %v", err)
	}

	// Cancel a running turn.
	done := make(chan string, 1)
	go func() {
		stop, _ := c.Prompt(ctx, sid, []acp.ContentBlock{acp.TextBlock("wait")})
		done <- stop
	}()
	time.Sleep(200 * time.Millisecond)
	if err := c.Cancel(sid); err != nil {
		t.Fatal(err)
	}
	select {
	case stop := <-done:
		if stop != acp.StopCancelled {
			t.Errorf("cancelled stop = %s", stop)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel not honored")
	}

	// A prompt error is returned as an error.
	if _, err := c.Prompt(ctx, sid, []acp.ContentBlock{acp.TextBlock("fail")}); err == nil {
		t.Error("fail prompt: no error")
	}
}

func TestClientAuthAndExit(t *testing.T) {
	h := &fakeHost{files: map[string]string{}, dir: t.TempDir()}
	c := startFake(t, h, "GOI_FAKE_AGENT_AUTH=1")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.NewSession(ctx, "/p", nil); !acp.IsAuthRequired(err) {
		t.Fatalf("expected auth required, got %v", err)
	}
	if err := c.Authenticate(ctx, "login"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.NewSession(ctx, "/p", nil); err != nil {
		t.Fatal(err)
	}
	c.Close()
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
	if _, err := c.NewSession(ctx, "/p", nil); err == nil {
		t.Error("call after close succeeded")
	}
}

func TestStartFailure(t *testing.T) {
	h := &fakeHost{files: map[string]string{}}
	_, err := Start(Options{Command: Command{Path: filepath.Join(t.TempDir(), "missing")}, Host: h})
	if err == nil {
		t.Fatal("no error for a missing program")
	}
	// A program that exits at once reports its stderr.
	if runtime.GOOS == "windows" {
		return
	}
	c, err := Start(Options{Command: Command{Path: "/bin/sh", Args: []string{"-c", "echo boom >&2; exit 7"}}, Host: h})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = c.Initialize(ctx)
	if err == nil || !strings.Contains(err.Error(), "code 7") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
}
