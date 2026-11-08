package rpc

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/terminal"
)

// TermOutputEvent is the payload of term.output.
type TermOutputEvent struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

// TermExitEvent is the payload of term.exit.
type TermExitEvent struct {
	ID   string `json:"id"`
	Code int    `json:"code"`
}

// TermInfo describes an opened terminal.
type TermInfo struct {
	ID    string `json:"id"`
	Shell string `json:"shell"`
	Cwd   string `json:"cwd"`
}

// termSession owns the project's terminals and batches their output.
type termSession struct {
	app *App
	mgr *terminal.Manager

	mu       sync.Mutex
	batchers map[string]*outputBatcher
}

func newTermSession(a *App) *termSession {
	ts := &termSession{app: a, batchers: map[string]*outputBatcher{}}
	ts.mgr = terminal.NewManager(ts.output, ts.exit)
	return ts
}

func (ts *termSession) batcher(id string) *outputBatcher {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	b := ts.batchers[id]
	if b == nil {
		b = newOutputBatcher(func(chunks []OutputChunk) {
			data := ""
			for _, c := range chunks {
				data += c.Text
			}
			ts.app.bus.Emit(events.TopicTermOutput, TermOutputEvent{ID: id, Data: data})
		})
		ts.batchers[id] = b
	}
	return b
}

func (ts *termSession) output(id string, data []byte) {
	ts.batcher(id).Write("pty", data)
}

func (ts *termSession) exit(id string, code int) {
	ts.batcher(id).Close()
	ts.mu.Lock()
	delete(ts.batchers, id)
	ts.mu.Unlock()
	ts.app.bus.Emit(events.TopicTermExit, TermExitEvent{ID: id, Code: code})
}

func (ts *termSession) close() { ts.mgr.CloseAll() }

// TerminalService runs integrated terminals for the open project.
type TerminalService struct {
	app *App
}

func (s *TerminalService) session() (*termSession, string, error) {
	sess, err := s.app.current()
	if err != nil {
		return nil, "", err
	}
	if sess.terms == nil {
		return nil, "", errNoProject
	}
	return sess.terms, sess.root, nil
}

// Open starts a shell in the project root with the project environment.
func (s *TerminalService) Open(cols, rows int) (TermInfo, error) {
	ts, root, err := s.session()
	if err != nil {
		return TermInfo{}, err
	}
	cfg := s.app.settings.Effective().Terminal
	shell := terminal.DefaultShell(cfg.Shell)
	env := os.Environ()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if r, err := s.app.runner(ctx); err == nil {
		env = r.Environ()
	}
	cancel()
	args := terminal.LoginArgs(shell)
	if cfg.ShellIntegration {
		a, e, err := terminal.Integration(filepath.Join(s.app.cacheDir, "shell"), shell, env)
		if err != nil {
			s.app.log.Warn("shell integration", "err", err)
		} else {
			args, env = a, e
		}
	}
	id, err := ts.mgr.Open(terminal.Spec{Shell: shell, Args: args, Dir: root, Env: terminal.Environ(env), Cols: cols, Rows: rows})
	if err != nil {
		return TermInfo{}, err
	}
	return TermInfo{ID: id, Shell: shell, Cwd: root}, nil
}

// Write sends keyboard input to a terminal.
func (s *TerminalService) Write(id, data string) error {
	ts, _, err := s.session()
	if err != nil {
		return err
	}
	return ts.mgr.Write(id, []byte(data))
}

// Resize changes a terminal's size.
func (s *TerminalService) Resize(id string, cols, rows int) error {
	ts, _, err := s.session()
	if err != nil {
		return err
	}
	return ts.mgr.Resize(id, cols, rows)
}

// Close kills a terminal's shell.
func (s *TerminalService) Close(id string) error {
	ts, _, err := s.session()
	if err != nil {
		return err
	}
	err = ts.mgr.Close(id)
	if err == terminal.ErrNoTerminal {
		return nil
	}
	return err
}

// Count is the number of open terminals.
func (s *TerminalService) Count() int {
	ts, _, err := s.session()
	if err != nil {
		return 0
	}
	return ts.mgr.Count()
}
