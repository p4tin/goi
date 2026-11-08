package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type sink struct {
	mu   sync.Mutex
	out  strings.Builder
	exit chan int
}

func newSink() *sink { return &sink{exit: make(chan int, 1)} }

func (s *sink) output(_ string, b []byte) {
	s.mu.Lock()
	s.out.Write(b)
	s.mu.Unlock()
}

func (s *sink) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.String()
}

func (s *sink) waitFor(t *testing.T, text string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(s.text(), text) {
		if time.Now().After(deadline) {
			t.Fatalf("no %q in terminal output: %q", text, s.text())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func cleanEnv(t *testing.T) []string {
	home := t.TempDir()
	return []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TERM=xterm-256color", "LANG=C"}
}

func TestPTYRoundTripAndExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newSink()
	m := NewManager(s.output, func(_ string, code int) { s.exit <- code })
	id, err := m.Open(Spec{Shell: "/bin/sh", Dir: t.TempDir(), Env: cleanEnv(t), Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	if m.Count() != 1 {
		t.Fatal("terminal not tracked")
	}
	if err := m.Resize(id, 120, 40); err != nil {
		t.Fatal(err)
	}
	m.Write(id, []byte("stty size; echo hi_$((40+2))\n"))
	s.waitFor(t, "40 120")
	s.waitFor(t, "hi_42")
	m.Write(id, []byte("exit 3\n"))
	select {
	case code := <-s.exit:
		if code != 3 {
			t.Fatalf("exit code = %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no exit")
	}
	if m.Count() != 0 {
		t.Fatal("exited terminal still tracked")
	}
	if err := m.Write(id, []byte("x")); err != ErrNoTerminal {
		t.Fatalf("write after exit = %v", err)
	}
}

func TestCloseKillsShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newSink()
	m := NewManager(s.output, func(string, int) {})
	id, err := m.Open(Spec{Shell: "/bin/sh", Env: cleanEnv(t)})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { m.CloseAll(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("CloseAll hung")
	}
	if _, err := m.Open(Spec{Shell: "/bin/sh"}); err == nil {
		t.Fatal("open after CloseAll should fail")
	}
	_ = id
}

func integrationShell(t *testing.T, name, probe string) {
	t.Helper()
	shell, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not installed", name)
	}
	dir := t.TempDir()
	args, env, err := Integration(dir, shell, cleanEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	s := newSink()
	m := NewManager(s.output, func(string, int) {})
	t.Cleanup(m.CloseAll)
	work := t.TempDir()
	id, err := m.Open(Spec{Shell: shell, Args: args, Dir: work, Env: Environ(env), Cols: 120, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	// OSC 7 reports the working directory at each prompt.
	s.waitFor(t, "\x1b]7;file://")
	m.Write(id, []byte(probe+"\n"))
	s.waitFor(t, "GOI_DEFINED")
	m.Write(id, []byte("cd "+filepath.Base(work)+"/.. && echo moved\n"))
	s.waitFor(t, "moved")
}

func TestZshIntegration(t *testing.T) {
	integrationShell(t, "zsh", `(( $+functions[_goi_go] )) && echo GOI_""DEFINED`)
}

func TestBashIntegration(t *testing.T) {
	integrationShell(t, "bash", `type _goi_go >/dev/null && complete -p go | grep -q _goi_go && echo GOI_""DEFINED`)
}

func TestIntegrationScripts(t *testing.T) {
	dir := t.TempDir()
	args, env, err := Integration(dir, "/usr/bin/zsh", []string{"HOME=/home/u", "ZDOTDIR=/home/u/.config/zsh"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "-l" || lookupEnv(env, "ZDOTDIR") != filepath.Join(dir, "zsh") || lookupEnv(env, "GOI_USER_ZDOTDIR") != "/home/u/.config/zsh" {
		t.Fatalf("zsh args/env = %v %v", args, env)
	}
	rc, _ := os.ReadFile(filepath.Join(dir, "zsh", ".zshrc"))
	if !strings.Contains(string(rc), "compdef _goi_go go") || !strings.Contains(string(rc), "test) opts=(") {
		t.Fatalf(".zshrc = %s", rc)
	}
	args, _, _ = Integration(dir, "/bin/bash", nil)
	if args[0] != "--rcfile" || !strings.HasSuffix(args[1], "goi.bashrc") {
		t.Fatalf("bash args = %v", args)
	}
	if args, _, _ := Integration(dir, "/usr/bin/fish", nil); strings.Join(args, " ") != "-l" {
		t.Fatalf("fish args = %v", args)
	}
	if ShellKind(`C:\Program Files\PowerShell\7\pwsh.exe`) != "pwsh" || ShellKind("/opt/x/elvish") != "other" {
		t.Fatal("ShellKind")
	}
	env = Environ([]string{"TERM=dumb"})
	if lookupEnv(env, "TERM") != "xterm-256color" || lookupEnv(env, "TERM_PROGRAM") != "Goi" {
		t.Fatalf("Environ = %v", env)
	}
}

func TestDefaultShell(t *testing.T) {
	if DefaultShell("/custom/sh") != "/custom/sh" {
		t.Fatal("setting wins")
	}
	if runtime.GOOS != "windows" {
		t.Setenv("SHELL", "/bin/testsh")
		if DefaultShell("") != "/bin/testsh" {
			t.Fatal("$SHELL")
		}
	}
}
