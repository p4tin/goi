package agent

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/p4tin/goi/internal/acp"
	"github.com/p4tin/goi/internal/process"
)

// DefaultOutputLimit caps a terminal's retained output when the agent gives
// no outputByteLimit.
const DefaultOutputLimit = 1 << 20

// ErrNoTerminal means the terminal id is unknown (or released).
var ErrNoTerminal = errors.New("unknown terminal")

// TerminalSpec is a command an agent asked to run.
type TerminalSpec struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
	Limit   int64
}

// Terminals runs agent commands (terminal/* requests) and keeps their output.
type Terminals struct {
	prefix   string
	onOutput func(id string, data []byte, exit *acp.ExitStatus)

	mu   sync.Mutex
	next int
	m    map[string]*term
}

type term struct {
	p     *process.Process
	limit int64

	mu        sync.Mutex
	buf       []byte
	truncated bool
	exit      *acp.ExitStatus
	done      chan struct{}
}

// NewTerminals returns a manager whose ids start with prefix. onOutput (may
// be nil) sees every output chunk and, last, the exit status.
func NewTerminals(prefix string, onOutput func(id string, data []byte, exit *acp.ExitStatus)) *Terminals {
	return &Terminals{prefix: prefix, onOutput: onOutput, m: map[string]*term{}}
}

// shellCommand runs a whole command line given without arguments through
// the system shell: agents often send "go test ./..." as the command.
func shellCommand(command string, args []string) (string, []string) {
	if len(args) > 0 || !strings.ContainsAny(command, " \t|&;<>$*?") {
		return command, args
	}
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", command}
	}
	return "/bin/sh", []string{"-c", command}
}

// Create starts spec and returns its id.
func (ts *Terminals) Create(spec TerminalSpec) (string, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return "", errors.New("terminal: empty command")
	}
	limit := spec.Limit
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	ts.mu.Lock()
	ts.next++
	id := fmt.Sprintf("%st%d", ts.prefix, ts.next)
	ts.mu.Unlock()

	t := &term{limit: limit, done: make(chan struct{})}
	path, args := shellCommand(spec.Command, spec.Args)
	p, err := process.Start(process.Spec{Path: path, Args: args, Dir: spec.Dir, Env: spec.Env}, func(_ string, data []byte) {
		t.append(data)
		if ts.onOutput != nil {
			ts.onOutput(id, data, nil)
		}
	})
	if err != nil {
		return "", err
	}
	_ = p.CloseStdin()
	t.p = p
	ts.mu.Lock()
	ts.m[id] = t
	ts.mu.Unlock()
	go func() {
		info := p.Wait()
		st := &acp.ExitStatus{Signal: info.Signal}
		if info.Signal == "" {
			code := info.Code
			st.ExitCode = &code
		}
		t.mu.Lock()
		t.exit = st
		t.mu.Unlock()
		close(t.done)
		if ts.onOutput != nil {
			ts.onOutput(id, nil, st)
		}
	}()
	return id, nil
}

// append adds output, dropping whole characters from the start beyond the
// limit.
func (t *term) append(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, data...)
	if over := int64(len(t.buf)) - t.limit; over > 0 {
		cut := int(over)
		for cut < len(t.buf) && !utf8.RuneStart(t.buf[cut]) {
			cut++
		}
		t.buf = append([]byte(nil), t.buf[cut:]...)
		t.truncated = true
	}
}

func (ts *Terminals) get(id string) (*term, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	t := ts.m[id]
	if t == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoTerminal, id)
	}
	return t, nil
}

// Output returns the retained output and, once it ended, the exit status.
func (ts *Terminals) Output(id string) (acp.TerminalOutputResult, error) {
	t, err := ts.get(id)
	if err != nil {
		return acp.TerminalOutputResult{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return acp.TerminalOutputResult{Output: strings.ToValidUTF8(string(t.buf), "�"), Truncated: t.truncated, ExitStatus: t.exit}, nil
}

// Wait blocks until the command ends or ctx is done.
func (ts *Terminals) Wait(ctx context.Context, id string) (acp.ExitStatus, error) {
	t, err := ts.get(id)
	if err != nil {
		return acp.ExitStatus{}, err
	}
	select {
	case <-t.done:
		t.mu.Lock()
		defer t.mu.Unlock()
		return *t.exit, nil
	case <-ctx.Done():
		return acp.ExitStatus{}, ctx.Err()
	}
}

// Kill stops the command (it stays queryable until released).
func (ts *Terminals) Kill(id string) error {
	t, err := ts.get(id)
	if err != nil {
		return err
	}
	t.p.Kill()
	return nil
}

// Release kills the command if it still runs and forgets the terminal.
func (ts *Terminals) Release(id string) error {
	ts.mu.Lock()
	t := ts.m[id]
	delete(ts.m, id)
	ts.mu.Unlock()
	if t == nil {
		return fmt.Errorf("%w: %s", ErrNoTerminal, id)
	}
	select {
	case <-t.done:
	default:
		t.p.Kill()
	}
	return nil
}

// CloseAll kills and forgets every terminal.
func (ts *Terminals) CloseAll() {
	ts.mu.Lock()
	m := ts.m
	ts.m = map[string]*term{}
	ts.mu.Unlock()
	for _, t := range m {
		t.p.Kill()
	}
}
