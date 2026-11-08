// Package git drives the git command line for one repository: status,
// contents, diffs and partial staging, commits, branches, merges and
// rebases, cherry-pick/revert, stash, history and blame. Using the CLI keeps
// the user's configuration, credential helpers, SSH agent and hooks. The
// package has no UI knowledge; network commands are returned as specs for
// the caller to run with streaming output.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotRepo means the directory is not inside a git work tree.
var ErrNotRepo = errors.New("git: not a git repository")

// Error is a failed git command with its output.
type Error struct {
	Args   []string
	Stderr string
	Code   int
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.Code)
	}
	return "git " + firstArg(e.Args) + ": " + msg
}

func firstArg(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// Repo is a git work tree.
type Repo struct {
	// Root is the work tree root (symlinks as given by git).
	Root string
	// Bin is the git executable.
	Bin string
	// Env is the base environment for git commands.
	Env    []string
	gitDir string
}

// Open finds the repository containing dir. bin "" means "git" on PATH.
func Open(ctx context.Context, dir, bin string, env []string) (*Repo, error) {
	if bin == "" {
		bin = "git"
	}
	r := &Repo{Root: dir, Bin: bin, Env: env}
	out, err := r.run(ctx, nil, "rev-parse", "--show-toplevel", "--absolute-git-dir")
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) && strings.Contains(strings.ToLower(ge.Stderr), "not a git repository") {
			return nil, ErrNotRepo
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return nil, ErrNotRepo
	}
	r.Root = filepath.FromSlash(strings.TrimSpace(lines[0]))
	r.gitDir = filepath.FromSlash(strings.TrimSpace(lines[1]))
	return r, nil
}

// Init creates a repository in dir.
func Init(ctx context.Context, dir, bin string, env []string) error {
	if bin == "" {
		bin = "git"
	}
	r := &Repo{Root: dir, Bin: bin, Env: env}
	_, err := r.run(ctx, nil, "init")
	return err
}

// GitDir is the absolute .git directory.
func (r *Repo) GitDir() string { return r.gitDir }

// environ is the environment for git: never prompt, stable messages.
func (r *Repo) environ(extra ...string) []string {
	env := r.Env
	if env == nil {
		env = os.Environ()
	}
	env = append(append([]string{}, env...), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "GIT_PAGER=cat")
	return append(env, extra...)
}

// baseArgs precede every git command.
var baseArgs = []string{"-c", "core.quotepath=off", "-c", "color.ui=never", "-c", "advice.detachedHead=false"}

// Command returns a git command for args with the repo environment (used
// for long-running network commands the caller streams).
func (r *Repo) Command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.Bin, append(append([]string{}, baseArgs...), args...)...)
	cmd.Dir = r.Root
	cmd.Env = r.environ()
	return cmd
}

func (r *Repo) run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	return r.runEnv(ctx, stdin, nil, args...)
}

func (r *Repo) runEnv(ctx context.Context, stdin io.Reader, extraEnv []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.Bin, append(append([]string{}, baseArgs...), args...)...)
	cmd.Dir = r.Root
	cmd.Env = r.environ(extraEnv...)
	cmd.Stdin = stdin
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := errb.String()
			if strings.TrimSpace(stderr) == "" {
				stderr = out.String() // some commands report conflicts on stdout
			}
			return out.Bytes(), &Error{Args: args, Stderr: stderr, Code: ee.ExitCode()}
		}
		return nil, fmt.Errorf("git: %w", err)
	}
	return out.Bytes(), nil
}

// abs makes a repo-relative slash path absolute.
func (r *Repo) abs(rel string) string { return filepath.Join(r.Root, filepath.FromSlash(rel)) }

// rel makes an absolute path repo-relative (slash form) for pathspecs.
func (r *Repo) rel(p string) string {
	if !filepath.IsAbs(p) {
		return filepath.ToSlash(p)
	}
	if rel, err := filepath.Rel(r.Root, p); err == nil {
		return filepath.ToSlash(rel)
	}
	// The root may be a symlink-resolved form of p's prefix.
	if real, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		if rel, err := filepath.Rel(r.Root, filepath.Join(real, filepath.Base(p))); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(p)
}

func (r *Repo) rels(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = r.rel(p)
	}
	return out
}

// Op is an operation in progress.
type Op string

const (
	OpNone       Op = ""
	OpMerge      Op = "merge"
	OpRebase     Op = "rebase"
	OpCherryPick Op = "cherry-pick"
	OpRevert     Op = "revert"
)

// CurrentOp detects a merge, rebase, cherry-pick or revert in progress.
func (r *Repo) CurrentOp() Op {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(r.gitDir, name))
		return err == nil
	}
	switch {
	case exists("rebase-merge") || exists("rebase-apply"):
		return OpRebase
	case exists("CHERRY_PICK_HEAD"):
		return OpCherryPick
	case exists("REVERT_HEAD"):
		return OpRevert
	case exists("MERGE_HEAD"):
		return OpMerge
	}
	return OpNone
}
