package rpc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/git"
	"github.com/p4tin/goi/internal/process"
	"github.com/p4tin/goi/internal/vfs"
)

// Git op states (git.op).
const (
	GitOpRunning = "running"
	GitOpDone    = "done"
	GitOpFailed  = "failed"
)

// GitState is the payload of git.status.
type GitState struct {
	Repo   bool       `json:"repo"`
	Status git.Status `json:"status"`
}

// GitOutputEvent is the payload of git.output.
type GitOutputEvent struct {
	OpID   string        `json:"opId"`
	Chunks []OutputChunk `json:"chunks"`
}

// GitOpEvent is the payload of git.op.
type GitOpEvent struct {
	OpID     string `json:"opId"`
	Title    string `json:"title"`
	State    string `json:"state"`
	Error    string `json:"error"`
	Rejected bool   `json:"rejected"`
}

// gitRefreshDelay coalesces file events before re-running git status.
const gitRefreshDelay = 400 * time.Millisecond

var errNoRepo = errors.New("the project is not in a git repository")

// gitSession owns the project's repository handle and status cache.
type gitSession struct {
	app  *App
	win  *window
	root string

	mu      sync.Mutex
	repo    *git.Repo
	state   GitState
	timer   *time.Timer
	cancels []func()
	ops     map[string]*process.Process
	opSeq   int
	closed  bool
}

func startGit(win *window, root string, w vfs.Watcher) *gitSession {
	gs := &gitSession{app: win.app, win: win, root: root, ops: map[string]*process.Process{}}
	gs.cancels = append(gs.cancels, w.Subscribe(func([]vfs.Event) { gs.schedule() }))
	gs.open(w)
	return gs
}

func (gs *gitSession) bin() string { return gs.win.settings.Effective().Git.Path }

// open (re)discovers the repository and watches its index and refs.
func (gs *gitSession) open(w vfs.Watcher) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := git.Open(ctx, gs.root, gs.bin(), nil)
	if err != nil {
		if !errors.Is(err, git.ErrNotRepo) {
			gs.app.log.Warn("git", "err", err)
		}
		gs.mu.Lock()
		gs.repo = nil
		gs.state = GitState{Status: git.Status{Files: []git.FileStatus{}}}
		gs.mu.Unlock()
		gs.win.bus.Emit(events.TopicGitStatus, gs.state)
		return
	}
	gs.mu.Lock()
	gs.repo = r
	gs.mu.Unlock()
	if w != nil {
		// .git is excluded from the project watcher; watch what git touches
		// on commits, checkouts, staging and fetches.
		for _, name := range []string{"index", "HEAD", "FETCH_HEAD", "MERGE_HEAD", filepath.Join("logs", "HEAD")} {
			if c, err := w.WatchFile(filepath.Join(r.GitDir(), name), gs.schedule); err == nil {
				gs.mu.Lock()
				gs.cancels = append(gs.cancels, c)
				gs.mu.Unlock()
			}
		}
	}
	gs.refresh()
}

func (gs *gitSession) schedule() {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if gs.closed || gs.repo == nil {
		return
	}
	if gs.timer != nil {
		gs.timer.Stop()
	}
	gs.timer = time.AfterFunc(gitRefreshDelay, gs.refresh)
}

// refresh re-reads the status and emits it.
func (gs *gitSession) refresh() {
	r := gs.current()
	if r == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := r.Status(ctx)
	if err != nil {
		gs.app.log.Warn("git status", "err", err)
		return
	}
	gs.mu.Lock()
	if gs.closed {
		gs.mu.Unlock()
		return
	}
	gs.state = GitState{Repo: true, Status: st}
	gs.mu.Unlock()
	gs.win.bus.Emit(events.TopicGitStatus, gs.state)
}

func (gs *gitSession) current() *git.Repo {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.repo
}

func (gs *gitSession) close() {
	gs.mu.Lock()
	gs.closed = true
	if gs.timer != nil {
		gs.timer.Stop()
	}
	cancels := gs.cancels
	ops := make([]*process.Process, 0, len(gs.ops))
	for _, p := range gs.ops {
		ops = append(ops, p)
	}
	gs.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	for _, p := range ops {
		p.Kill()
	}
}

// startOp runs a long git command (fetch, pull, push) with streamed output.
func (gs *gitSession) startOp(title string, args []string, after func(output string, info process.ExitInfo)) (string, error) {
	r := gs.current()
	if r == nil {
		return "", errNoRepo
	}
	gs.mu.Lock()
	gs.opSeq++
	id := "git-" + strconv.Itoa(gs.opSeq)
	gs.mu.Unlock()
	cmd := r.Command(context.Background(), args...)
	var outMu sync.Mutex
	var all strings.Builder
	b := newOutputBatcher(func(chunks []OutputChunk) {
		gs.win.bus.Emit(events.TopicGitOutput, GitOutputEvent{OpID: id, Chunks: chunks})
	})
	b.Write(streamSystem, []byte("git "+strings.Join(args, " ")+"\n"))
	p, err := process.Start(process.Spec{Path: cmd.Path, Args: cmd.Args[1:], Env: cmd.Env, Dir: cmd.Dir, KillAfter: 3 * time.Second}, func(stream string, data []byte) {
		outMu.Lock()
		all.Write(data)
		outMu.Unlock()
		b.Write(stream, data)
	})
	if err != nil {
		return "", err
	}
	gs.mu.Lock()
	gs.ops[id] = p
	gs.mu.Unlock()
	gs.win.bus.Emit(events.TopicGitOp, GitOpEvent{OpID: id, Title: title, State: GitOpRunning})
	go func() {
		info := p.Wait()
		b.Close()
		gs.mu.Lock()
		delete(gs.ops, id)
		gs.mu.Unlock()
		outMu.Lock()
		output := all.String()
		outMu.Unlock()
		ev := GitOpEvent{OpID: id, Title: title, State: GitOpDone}
		if info.Code != 0 || info.Signal != "" {
			ev.State = GitOpFailed
			ev.Error = lastLines(output, 6)
			ev.Rejected = git.IsRejectedPush(output)
		}
		if after != nil {
			after(output, info)
		}
		gs.refresh()
		gs.win.bus.Emit(events.TopicGitOp, ev)
	}()
	return id, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var keep []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		// Drop carriage-return progress lines.
		if i := strings.LastIndex(l, "\r"); i >= 0 {
			l = l[i+1:]
		}
		if l != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return strings.Join(keep, "\n")
}

// GitService exposes git for the open project.
type GitService struct {
	app *App
}

func (s *GitService) session(ctx context.Context) (*gitSession, error) {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return nil, err
	}
	if sess.git == nil {
		return nil, errNoProject
	}
	return sess.git, nil
}

func (s *GitService) repo(ctx context.Context) (*gitSession, *git.Repo, error) {
	gs, err := s.session(ctx)
	if err != nil {
		return nil, nil, err
	}
	r := gs.current()
	if r == nil {
		return nil, nil, errNoRepo
	}
	return gs, r, nil
}

func gitCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}

// mutate runs fn and refreshes the status afterwards.
func (s *GitService) mutate(ctx context.Context, fn func(ctx context.Context, r *git.Repo) error) error {
	gs, r, err := s.repo(ctx)
	if err != nil {
		return err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	err = fn(gctx, r)
	gs.refresh()
	return err
}

// State returns the cached status (Repo false outside a repository).
func (s *GitService) State(ctx context.Context) GitState {
	gs, err := s.session(ctx)
	if err != nil {
		return GitState{Status: git.Status{Files: []git.FileStatus{}}}
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	st := gs.state
	if st.Status.Files == nil {
		st.Status.Files = []git.FileStatus{}
	}
	return st
}

// Refresh re-reads the status now (window focus, after terminal use).
func (s *GitService) Refresh(ctx context.Context) {
	if gs, err := s.session(ctx); err == nil {
		gs.refresh()
	}
}

// Init creates a repository at the project root.
func (s *GitService) Init(ctx context.Context) error {
	gs, err := s.session(ctx)
	if err != nil {
		return err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	if err := git.Init(gctx, gs.root, gs.bin(), nil); err != nil {
		return err
	}
	if sess, err := s.app.sessionFor(ctx); err == nil {
		gs.open(sess.watcher)
	}
	return nil
}

// Show returns a file's content at rev (":0" index, "HEAD", a hash, ":2"…).
// Missing files return "" and found=false.
func (s *GitService) Show(ctx context.Context, rev, path string) (ShowResult, error) {
	_, r, err := s.repo(ctx)
	if err != nil {
		return ShowResult{}, err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	text, err := r.Show(gctx, rev, path)
	if errors.Is(err, git.ErrNotFound) {
		return ShowResult{}, nil
	}
	if err != nil {
		return ShowResult{}, err
	}
	return ShowResult{Text: text, Found: true}, nil
}

// ShowResult is file content at a revision.
type ShowResult struct {
	Text  string `json:"text"`
	Found bool   `json:"found"`
}

// Diff returns unified diff text parsed into files.
func (s *GitService) Diff(ctx context.Context, path string, staged bool, rev, commit string) ([]git.FileDiff, error) {
	_, r, err := s.repo(ctx)
	if err != nil {
		return []git.FileDiff{}, err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	text, err := r.Diff(gctx, git.DiffSpec{Path: path, Staged: staged, Rev: rev, Commit: commit})
	if err != nil {
		return []git.FileDiff{}, err
	}
	return git.ParseDiff(text), nil
}

// ApplyPatch applies a patch to the index (cached) or the work tree.
func (s *GitService) ApplyPatch(ctx context.Context, patch string, cached, reverse bool) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.ApplyPatch(ctx, patch, cached, reverse) })
}

// Stage adds paths to the index.
func (s *GitService) Stage(ctx context.Context, paths []string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Stage(ctx, paths) })
}

// Unstage removes paths from the index.
func (s *GitService) Unstage(ctx context.Context, paths []string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Unstage(ctx, paths) })
}

// Rollback discards changes; untracked files are moved to the trash.
func (s *GitService) Rollback(ctx context.Context, paths []string) error {
	gs, _, err := s.repo(ctx)
	if err != nil {
		return err
	}
	untracked := map[string]bool{}
	gs.mu.Lock()
	for _, f := range gs.state.Status.Files {
		if f.Untracked {
			untracked[f.Path] = true
		}
	}
	gs.mu.Unlock()
	var tracked []string
	for _, p := range paths {
		if untracked[p] {
			if err := vfs.Trash(p); err != nil {
				if rmErr := os.Remove(p); rmErr != nil {
					return err
				}
			}
			continue
		}
		tracked = append(tracked, p)
	}
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Discard(ctx, tracked) })
}

// Commit commits the index.
func (s *GitService) Commit(ctx context.Context, message string, amend bool) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Commit(ctx, message, amend) })
}

// LastMessage is HEAD's message (for amend).
func (s *GitService) LastMessage(ctx context.Context) string {
	_, r, err := s.repo(ctx)
	if err != nil {
		return ""
	}
	gctx, cancel := gitCtx()
	defer cancel()
	return r.LastMessage(gctx)
}

// Branches lists branches.
func (s *GitService) Branches(ctx context.Context) ([]git.Branch, error) {
	_, r, err := s.repo(ctx)
	if err != nil {
		return []git.Branch{}, err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	return r.Branches(gctx)
}

// Checkout switches branch (newBranch creates one from ref or HEAD).
func (s *GitService) Checkout(ctx context.Context, ref, newBranch string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Checkout(ctx, ref, newBranch) })
}

// RenameBranch renames a local branch.
func (s *GitService) RenameBranch(ctx context.Context, from, to string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.RenameBranch(ctx, from, to) })
}

// DeleteBranch deletes a local branch; "unmerged" errors mean force is
// needed.
func (s *GitService) DeleteBranch(ctx context.Context, name string, force bool) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.DeleteBranch(ctx, name, force) })
}

// Merge merges ref into the current branch.
func (s *GitService) Merge(ctx context.Context, ref string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Merge(ctx, ref) })
}

// Rebase rebases the current branch onto ref.
func (s *GitService) Rebase(ctx context.Context, onto string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Rebase(ctx, onto) })
}

// CherryPick applies a commit.
func (s *GitService) CherryPick(ctx context.Context, hash string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.CherryPick(ctx, hash) })
}

// Revert reverts a commit.
func (s *GitService) Revert(ctx context.Context, hash string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Revert(ctx, hash) })
}

// Reset moves the current branch (mode soft, mixed, hard).
func (s *GitService) Reset(ctx context.Context, rev, mode string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Reset(ctx, rev, mode) })
}

// Continue, Skip and Abort control the operation in progress.
func (s *GitService) Continue(ctx context.Context) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Continue(ctx) })
}

// Skip skips the current commit of a rebase or cherry-pick.
func (s *GitService) Skip(ctx context.Context) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Skip(ctx) })
}

// Abort aborts the operation in progress.
func (s *GitService) Abort(ctx context.Context) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.Abort(ctx) })
}

// AcceptSide resolves a conflicted file with "ours" or "theirs" and stages it.
func (s *GitService) AcceptSide(ctx context.Context, path, side string) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error {
		if err := r.CheckoutVersion(ctx, path, side); err != nil {
			return err
		}
		return r.Stage(ctx, []string{path})
	})
}

// Stash stashes changes.
func (s *GitService) Stash(ctx context.Context, message string, untracked bool) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.StashPush(ctx, message, untracked) })
}

// Stashes lists stashes.
func (s *GitService) Stashes(ctx context.Context) ([]git.Stash, error) {
	_, r, err := s.repo(ctx)
	if err != nil {
		return []git.Stash{}, err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	return r.Stashes(gctx)
}

// StashApply applies (or pops) stash i.
func (s *GitService) StashApply(ctx context.Context, i int, pop bool) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.StashApply(ctx, i, pop) })
}

// StashDrop deletes stash i.
func (s *GitService) StashDrop(ctx context.Context, i int) error {
	return s.mutate(ctx, func(ctx context.Context, r *git.Repo) error { return r.StashDrop(ctx, i) })
}

// StashDiff is the patch of stash i, parsed.
func (s *GitService) StashDiff(ctx context.Context, i int) ([]git.FileDiff, error) {
	_, r, err := s.repo(ctx)
	if err != nil {
		return []git.FileDiff{}, err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	text, err := r.StashDiff(gctx, i)
	if err != nil {
		return []git.FileDiff{}, err
	}
	return git.ParseDiff(text), nil
}

// Log lists commits.
func (s *GitService) Log(ctx context.Context, q git.LogQuery) ([]git.Commit, error) {
	_, r, err := s.repo(ctx)
	if err != nil {
		return []git.Commit{}, err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	return r.Log(gctx, q)
}

// CommitFiles lists the files a commit changed.
func (s *GitService) CommitFiles(ctx context.Context, hash string) ([]git.ChangedFile, error) {
	_, r, err := s.repo(ctx)
	if err != nil {
		return []git.ChangedFile{}, err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	return r.CommitFiles(gctx, hash)
}

// Blame annotates a file using the given (possibly unsaved) contents.
func (s *GitService) Blame(ctx context.Context, path, contents string) ([]git.BlameLine, error) {
	_, r, err := s.repo(ctx)
	if err != nil {
		return []git.BlameLine{}, err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	return r.Blame(gctx, path, &contents)
}

// AddToGitignore appends paths to .gitignore.
func (s *GitService) AddToGitignore(ctx context.Context, paths []string) error {
	return s.mutate(ctx, func(_ context.Context, r *git.Repo) error { return r.AddToGitignore(paths) })
}

// Remotes lists remote names.
func (s *GitService) Remotes(ctx context.Context) ([]string, error) {
	_, r, err := s.repo(ctx)
	if err != nil {
		return []string{}, err
	}
	gctx, cancel := gitCtx()
	defer cancel()
	return r.Remotes(gctx)
}

// Fetch fetches all remotes in the background; returns the op id.
func (s *GitService) Fetch(ctx context.Context) (string, error) {
	gs, err := s.session(ctx)
	if err != nil {
		return "", err
	}
	return gs.startOp("Fetch", git.FetchArgs(), nil)
}

// Pull pulls the current branch (rebase per setting) in the background.
func (s *GitService) Pull(ctx context.Context) (string, error) {
	gs, err := s.session(ctx)
	if err != nil {
		return "", err
	}
	return gs.startOp("Pull", git.PullArgs(gs.win.settings.Effective().Git.PullRebase), nil)
}

// Push pushes the current branch in the background (sets the upstream when
// missing). force uses --force-with-lease.
func (s *GitService) Push(ctx context.Context, force bool) (string, error) {
	gs, err := s.session(ctx)
	if err != nil {
		return "", err
	}
	gs.mu.Lock()
	st := gs.state.Status
	gs.mu.Unlock()
	if st.Detached {
		return "", errors.New("HEAD is detached: check out a branch before pushing")
	}
	return gs.startOp("Push", git.PushArgs(st, "", force), nil)
}

// StopOp stops a running fetch/pull/push.
func (s *GitService) StopOp(ctx context.Context, id string) error {
	gs, err := s.session(ctx)
	if err != nil {
		return err
	}
	gs.mu.Lock()
	p := gs.ops[id]
	gs.mu.Unlock()
	if p == nil {
		return errors.New("the operation has finished")
	}
	p.Stop()
	return nil
}
