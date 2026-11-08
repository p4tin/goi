// Package terminal runs interactive shells in pseudo-terminals (ConPTY on
// Windows) for the integrated terminal, and writes shell-integration
// scripts that report the working directory (OSC 7) and complete `go`
// commands in zsh and bash.
package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	pty "github.com/aymanbagabas/go-pty"
)

// Spec describes a terminal to open.
type Spec struct {
	Shell string
	Args  []string
	Dir   string
	Env   []string
	Cols  int
	Rows  int
}

// Manager owns the open terminals.
type Manager struct {
	onOutput func(id string, data []byte)
	onExit   func(id string, code int)

	mu     sync.Mutex
	terms  map[string]*term
	seq    int
	closed bool
}

type term struct {
	p         pty.Pty
	cmd       *pty.Cmd
	done      chan struct{}
	closeOnce sync.Once
}

// closePty closes the pseudo-terminal once (go-pty's Close is not safe to
// call concurrently).
func (t *term) closePty() {
	t.closeOnce.Do(func() { _ = t.p.Close() })
}

// NewManager reports output and exits through the callbacks (called from
// the terminals' own goroutines).
func NewManager(onOutput func(id string, data []byte), onExit func(id string, code int)) *Manager {
	return &Manager{onOutput: onOutput, onExit: onExit, terms: map[string]*term{}}
}

// ErrNoTerminal is returned for unknown terminal ids.
var ErrNoTerminal = errors.New("terminal: no such terminal")

// Open starts spec's shell in a new pseudo-terminal.
func (m *Manager) Open(spec Spec) (string, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", errors.New("terminal: manager closed")
	}
	m.seq++
	id := "term-" + strconv.Itoa(m.seq)
	m.mu.Unlock()

	p, err := pty.New()
	if err != nil {
		return "", fmt.Errorf("terminal: %w", err)
	}
	cols, rows := spec.Cols, spec.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	_ = p.Resize(cols, rows)
	cmd := p.Command(spec.Shell, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	if err := cmd.Start(); err != nil {
		p.Close()
		return "", fmt.Errorf("terminal: start %s: %w", spec.Shell, err)
	}
	t := &term{p: p, cmd: cmd, done: make(chan struct{})}
	m.mu.Lock()
	m.terms[id] = t
	m.mu.Unlock()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 32*1024)
		for {
			n, err := p.Read(buf)
			if n > 0 && m.onOutput != nil {
				m.onOutput(id, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		err := cmd.Wait()
		code := 0
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		} else if err != nil {
			code = -1
		}
		// Let the reader drain what the shell printed last; on some systems
		// the master only reports EOF once closed.
		select {
		case <-readDone:
		case <-timeAfter():
		}
		t.closePty()
		<-readDone
		m.mu.Lock()
		delete(m.terms, id)
		m.mu.Unlock()
		close(t.done)
		if m.onExit != nil {
			m.onExit(id, code)
		}
	}()
	return id, nil
}

func (m *Manager) get(id string) (*term, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.terms[id]
	if !ok {
		return nil, ErrNoTerminal
	}
	return t, nil
}

// Write sends input to the terminal.
func (m *Manager) Write(id string, data []byte) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}
	_, err = t.p.Write(data)
	return err
}

// Resize changes the terminal size.
func (m *Manager) Resize(id string, cols, rows int) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}
	if cols < 2 || rows < 1 {
		return nil
	}
	return t.p.Resize(cols, rows)
}

// Close kills the terminal's shell.
func (m *Manager) Close(id string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	t.closePty()
	<-t.done
	return nil
}

// CloseAll kills every terminal.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	m.closed = true
	ids := make([]string, 0, len(m.terms))
	for id := range m.terms {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Close(id)
	}
}

// Count is the number of open terminals.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.terms)
}

// DefaultShell picks the user's shell: setting, then $SHELL (Unix) or pwsh,
// powershell, cmd (Windows), then /bin/sh.
func DefaultShell(setting string) string {
	if setting != "" {
		return setting
	}
	if runtime.GOOS == "windows" {
		for _, s := range []string{"pwsh.exe", "powershell.exe"} {
			if p, err := exec.LookPath(s); err == nil {
				return p
			}
		}
		if c := os.Getenv("ComSpec"); c != "" {
			return c
		}
		return "cmd.exe"
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	for _, s := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(s); err == nil {
			return s
		}
	}
	return "/bin/sh"
}

// ShellKind classifies a shell by its executable name.
func ShellKind(shell string) string {
	base := shell
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = trimExe(base)
	switch base {
	case "zsh", "bash", "fish", "pwsh", "powershell", "cmd", "sh", "dash", "ksh":
		return base
	}
	return "other"
}

func trimExe(s string) string {
	if len(s) > 4 && (s[len(s)-4:] == ".exe" || s[len(s)-4:] == ".EXE") {
		return s[:len(s)-4]
	}
	return s
}
