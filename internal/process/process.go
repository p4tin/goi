// Package process starts and supervises user programs (run configurations,
// builds, tests): output streaming, interactive stdin, and stopping the
// whole process group (interrupt first, kill after a grace period).
package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Streams reported to the output callback.
const (
	Stdout = "stdout"
	Stderr = "stderr"
)

// DefaultKillAfter is how long Stop waits after the interrupt before killing.
const DefaultKillAfter = 5 * time.Second

// Spec describes a process to start.
type Spec struct {
	Path string
	Args []string
	Env  []string
	Dir  string
	// StdinFile redirects stdin from a file; empty gives an interactive pipe.
	StdinFile string
	// KillAfter overrides DefaultKillAfter.
	KillAfter time.Duration
}

// ExitInfo describes how a process ended.
type ExitInfo struct {
	Code int `json:"code"`
	// Signal is set when the process was terminated by a signal (Unix).
	Signal string `json:"signal"`
	// Err is set when the process could not be waited for.
	Err string `json:"err"`
}

// Process is a running program.
type Process struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdinFile *os.File
	killAfter time.Duration
	done      chan struct{}

	mu       sync.Mutex
	exit     ExitInfo
	stopping bool
	timer    *time.Timer
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// Start starts spec. out receives output chunks (the slice is only valid
// during the call) from the process's own goroutines.
func Start(spec Spec, out func(stream string, data []byte)) (*Process, error) {
	if spec.Path == "" {
		return nil, errors.New("process: no command")
	}
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.Stdout = writerFunc(func(b []byte) (int, error) { out(Stdout, b); return len(b), nil })
	cmd.Stderr = writerFunc(func(b []byte) (int, error) { out(Stderr, b); return len(b), nil })
	// Don't hang on grandchildren that keep the output pipes open.
	cmd.WaitDelay = 2 * time.Second
	setGroup(cmd)

	p := &Process{cmd: cmd, killAfter: spec.KillAfter, done: make(chan struct{})}
	if p.killAfter <= 0 {
		p.killAfter = DefaultKillAfter
	}
	if spec.StdinFile != "" {
		f, err := os.Open(spec.StdinFile)
		if err != nil {
			return nil, fmt.Errorf("process: input file: %w", err)
		}
		cmd.Stdin = f
		p.stdinFile = f
	} else {
		w, err := cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("process: stdin: %w", err)
		}
		p.stdin = w
	}
	if err := cmd.Start(); err != nil {
		if p.stdinFile != nil {
			p.stdinFile.Close()
		}
		return nil, fmt.Errorf("process: start %s: %w", spec.Path, err)
	}
	go p.wait()
	return p, nil
}

func (p *Process) wait() {
	err := p.cmd.Wait()
	info := exitInfo(p.cmd.ProcessState, err)
	if p.stdinFile != nil {
		p.stdinFile.Close()
	}
	p.mu.Lock()
	p.exit = info
	if p.timer != nil {
		p.timer.Stop()
	}
	p.mu.Unlock()
	close(p.done)
}

func exitInfo(st *os.ProcessState, err error) ExitInfo {
	if st == nil {
		if err != nil {
			return ExitInfo{Code: -1, Err: err.Error()}
		}
		return ExitInfo{Code: -1}
	}
	info := ExitInfo{Code: st.ExitCode()}
	if sig := signalOf(st); sig != "" {
		info.Signal = sig
	}
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) && !errors.Is(err, exec.ErrWaitDelay) {
		info.Err = err.Error()
	}
	return info
}

// Pid is the process id.
func (p *Process) Pid() int { return p.cmd.Process.Pid }

// Write sends b to the process's stdin.
func (p *Process) Write(b []byte) error {
	if p.stdin == nil {
		return errors.New("process: stdin is redirected from a file")
	}
	_, err := p.stdin.Write(b)
	return err
}

// CloseStdin closes the interactive stdin (EOF for the program).
func (p *Process) CloseStdin() error {
	if p.stdin == nil {
		return nil
	}
	return p.stdin.Close()
}

// Stop interrupts the process group and kills it after the grace period.
// Calling Stop again while stopping kills immediately. On Windows, where
// GUI-started processes have no console to interrupt, Stop kills the tree.
func (p *Process) Stop() {
	select {
	case <-p.done:
		return
	default:
	}
	p.mu.Lock()
	again := p.stopping
	p.stopping = true
	p.mu.Unlock()
	if again || !canInterrupt {
		p.Kill()
		return
	}
	if err := interruptGroup(p.cmd); err != nil {
		p.Kill()
		return
	}
	p.mu.Lock()
	p.timer = time.AfterFunc(p.killAfter, p.Kill)
	p.mu.Unlock()
}

// Stopping reports whether Stop was called.
func (p *Process) Stopping() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopping
}

// Kill kills the whole process group.
func (p *Process) Kill() {
	select {
	case <-p.done:
		return
	default:
	}
	killGroup(p.cmd)
}

// Done is closed when the process has exited and its output is drained.
func (p *Process) Done() <-chan struct{} { return p.done }

// Wait blocks until the process exits and returns how it ended.
func (p *Process) Wait() ExitInfo {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exit
}
