package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Stage adds paths (including deletions) to the index.
func (r *Repo) Stage(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := r.run(ctx, nil, append([]string{"add", "-A", "--"}, r.rels(paths)...)...)
	return err
}

// Unstage removes paths from the index (keeping work tree changes).
func (r *Repo) Unstage(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if !r.hasHead(ctx) {
		_, err := r.run(ctx, nil, append([]string{"rm", "--cached", "-r", "-q", "--"}, r.rels(paths)...)...)
		return err
	}
	_, err := r.run(ctx, nil, append([]string{"restore", "--staged", "--"}, r.rels(paths)...)...)
	return err
}

// Discard restores tracked paths to HEAD in the index and work tree. Paths
// not in HEAD (added files) are unstaged and removed. Untracked files are
// the caller's to delete (Goi moves them to the trash).
func (r *Repo) Discard(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	var tracked, added []string
	for _, p := range paths {
		if r.hasHead(ctx) && r.inHead(ctx, p) {
			tracked = append(tracked, r.rel(p))
		} else {
			added = append(added, p)
		}
	}
	if len(tracked) > 0 {
		if _, err := r.run(ctx, nil, append([]string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}, tracked...)...); err != nil {
			return err
		}
	}
	if len(added) > 0 {
		_ = r.Unstage(ctx, added)
		for _, p := range added {
			if err := os.Remove(r.abs(r.rel(p))); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("git: discard %s: %w", p, err)
			}
		}
	}
	return nil
}

func (r *Repo) hasHead(ctx context.Context) bool {
	_, err := r.run(ctx, nil, "rev-parse", "--verify", "-q", "HEAD")
	return err == nil
}

func (r *Repo) inHead(ctx context.Context, p string) bool {
	_, err := r.run(ctx, nil, "cat-file", "-e", "HEAD:"+r.rel(p))
	return err == nil
}

// Commit commits the index. amend replaces the last commit.
func (r *Repo) Commit(ctx context.Context, message string, amend bool) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("git: commit message is empty")
	}
	args := []string{"commit", "-F", "-"}
	if amend {
		args = append(args, "--amend")
	}
	_, err := r.run(ctx, strings.NewReader(message), args...)
	return err
}

// LastMessage is the full message of HEAD ("" before the first commit).
func (r *Repo) LastMessage(ctx context.Context) string {
	out, err := r.run(ctx, nil, "log", "-1", "--format=%B")
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// Reset moves the current branch to rev (mode soft, mixed or hard).
func (r *Repo) Reset(ctx context.Context, rev, mode string) error {
	switch mode {
	case "soft", "mixed", "hard":
	default:
		return fmt.Errorf("git: unknown reset mode %q", mode)
	}
	_, err := r.run(ctx, nil, "reset", "--"+mode, rev)
	return err
}

// CherryPick applies a commit onto the current branch.
func (r *Repo) CherryPick(ctx context.Context, hash string) error {
	_, err := r.run(ctx, nil, "cherry-pick", hash)
	return err
}

// Revert creates a commit undoing hash.
func (r *Repo) Revert(ctx context.Context, hash string) error {
	_, err := r.run(ctx, nil, "revert", "--no-edit", hash)
	return err
}

// Merge merges ref into the current branch.
func (r *Repo) Merge(ctx context.Context, ref string) error {
	_, err := r.run(ctx, nil, "merge", "--no-edit", ref)
	return err
}

// Rebase rebases the current branch onto ref.
func (r *Repo) Rebase(ctx context.Context, onto string) error {
	_, err := r.runEnv(ctx, nil, []string{"GIT_EDITOR=true"}, "rebase", onto)
	return err
}

// Continue continues the operation in progress.
func (r *Repo) Continue(ctx context.Context) error { return r.opCmd(ctx, "--continue") }

// Skip skips the current commit of a rebase or cherry-pick.
func (r *Repo) Skip(ctx context.Context) error { return r.opCmd(ctx, "--skip") }

// Abort aborts the operation in progress.
func (r *Repo) Abort(ctx context.Context) error { return r.opCmd(ctx, "--abort") }

func (r *Repo) opCmd(ctx context.Context, flag string) error {
	op := r.CurrentOp()
	if op == OpNone {
		return errors.New("git: no merge, rebase, cherry-pick or revert in progress")
	}
	if op == OpMerge && flag == "--skip" {
		return errors.New("git: a merge can't be skipped")
	}
	if op == OpMerge && flag == "--continue" {
		// git merge --continue needs an editor; commit with the prepared message.
		_, err := r.runEnv(ctx, nil, []string{"GIT_EDITOR=true"}, "commit", "--no-edit")
		return err
	}
	_, err := r.runEnv(ctx, nil, []string{"GIT_EDITOR=true"}, string(op), flag)
	return err
}

// CheckoutVersion replaces a conflicted file with "ours" or "theirs".
func (r *Repo) CheckoutVersion(ctx context.Context, path, side string) error {
	if side != "ours" && side != "theirs" {
		return fmt.Errorf("git: unknown side %q", side)
	}
	_, err := r.run(ctx, nil, "checkout", "--"+side, "--", r.rel(path))
	return err
}

// AddToGitignore appends repo-relative patterns for paths to .gitignore.
func (r *Repo) AddToGitignore(paths []string) error {
	p := filepath.Join(r.Root, ".gitignore")
	existing, _ := os.ReadFile(p)
	have := map[string]bool{}
	for _, l := range strings.Split(string(existing), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var add []string
	for _, x := range paths {
		pat := "/" + r.rel(x)
		if st, err := os.Stat(x); err == nil && st.IsDir() {
			pat += "/"
		}
		if !have[pat] {
			add = append(add, pat)
			have[pat] = true
		}
	}
	if len(add) == 0 {
		return nil
	}
	text := string(existing)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += strings.Join(add, "\n") + "\n"
	return os.WriteFile(p, []byte(text), 0o644)
}

// ---- Network commands (returned as argument lists for streaming).

// FetchArgs fetches all remotes, pruning deleted branches.
func FetchArgs() []string { return []string{"fetch", "--all", "--prune", "--progress"} }

// PullArgs pulls the current branch (rebase or merge).
func PullArgs(rebase bool) []string {
	if rebase {
		return []string{"pull", "--rebase", "--progress"}
	}
	return []string{"pull", "--no-rebase", "--progress"}
}

// PushArgs pushes the current branch, setting the upstream when missing.
func PushArgs(st Status, remote string, force bool) []string {
	args := []string{"push", "--progress"}
	if force {
		args = append(args, "--force-with-lease")
	}
	if st.Upstream == "" && st.Branch != "" {
		if remote == "" {
			remote = "origin"
		}
		args = append(args, "-u", remote, st.Branch)
	}
	return args
}

// IsRejectedPush reports whether push output says the remote rejected a
// non-fast-forward update.
func IsRejectedPush(output string) bool {
	return strings.Contains(output, "[rejected]") || strings.Contains(output, "non-fast-forward") || strings.Contains(output, "fetch first")
}

// Remotes lists remote names.
func (r *Repo) Remotes(ctx context.Context) ([]string, error) {
	out, err := r.run(ctx, nil, "remote")
	if err != nil {
		return nil, err
	}
	names := strings.Fields(string(out))
	if names == nil {
		names = []string{}
	}
	return names, nil
}
