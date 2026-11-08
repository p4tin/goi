package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var ctx = context.Background()

// testEnv isolates git from the user's config.
func testEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	return append(os.Environ(),
		"HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Goi Test", "GIT_AUTHOR_EMAIL=goi@example.com",
		"GIT_COMMITTER_NAME=Goi Test", "GIT_COMMITTER_EMAIL=goi@example.com")
}

func sh(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, path, text string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a repo with one commit (a.txt, b.txt) on branch main.
func newRepo(t *testing.T) (*Repo, []string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	env := testEnv(t)
	sh(t, dir, env, "init", "-q", "-b", "main")
	write(t, filepath.Join(dir, "a.txt"), "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n")
	write(t, filepath.Join(dir, "b.txt"), "bee\n")
	sh(t, dir, env, "add", ".")
	sh(t, dir, env, "commit", "-q", "-m", "initial")
	r, err := Open(ctx, filepath.Join(dir), "", env)
	if err != nil {
		t.Fatal(err)
	}
	return r, env
}

func find(st Status, rel string) *FileStatus {
	for i := range st.Files {
		if st.Files[i].Rel == rel {
			return &st.Files[i]
		}
	}
	return nil
}

func TestOpenOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_, err := Open(ctx, t.TempDir(), "", testEnv(t))
	if !errors.Is(err, ErrNotRepo) {
		t.Fatalf("want ErrNotRepo, got %v", err)
	}
}

func TestStatusCodes(t *testing.T) {
	r, env := newRepo(t)
	write(t, r.abs("a.txt"), "one\nTWO\n")
	write(t, r.abs("new.txt"), "n\n")
	write(t, r.abs("dir/deep.txt"), "d\n")
	sh(t, r.Root, env, "mv", "b.txt", "c.txt")
	write(t, r.abs("staged.txt"), "s\n")
	sh(t, r.Root, env, "add", "staged.txt")
	st, err := r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" || st.Head == "" || st.Detached || st.Op != OpNone {
		t.Fatalf("branch info = %+v", st)
	}
	if f := find(st, "a.txt"); f == nil || f.Index != "." || f.Work != "M" || f.Staged() || !f.Unstaged() {
		t.Errorf("a.txt = %+v", f)
	}
	if f := find(st, "new.txt"); f == nil || !f.Untracked {
		t.Errorf("new.txt = %+v", f)
	}
	if f := find(st, "dir/deep.txt"); f == nil || !f.Untracked || f.Path != filepath.Join(r.Root, "dir", "deep.txt") {
		t.Errorf("deep untracked = %+v", f)
	}
	if f := find(st, "c.txt"); f == nil || f.Index != "R" || f.Orig != r.abs("b.txt") {
		t.Errorf("rename = %+v", f)
	}
	if f := find(st, "staged.txt"); f == nil || f.Index != "A" || !f.Staged() {
		t.Errorf("staged = %+v", f)
	}
}

func TestPartialStagingRoundTrip(t *testing.T) {
	r, _ := newRepo(t)
	// b.txt with 30 lines between two edits gives two separate hunks.
	big := ""
	for i := 0; i < 30; i++ {
		big += "line\n"
	}
	write(t, r.abs("b.txt"), "BEE\n"+big+"END\n")
	r.Stage(ctx, []string{r.abs("b.txt")})
	r.Commit(ctx, "big b", false)
	write(t, r.abs("b.txt"), "BEE changed\n"+big+"END changed\n")
	text, err := r.Diff(ctx, DiffSpec{Path: r.abs("b.txt")})
	if err != nil {
		t.Fatal(err)
	}
	files := ParseDiff(text)
	if len(files) != 1 || len(files[0].Hunks) != 2 {
		t.Fatalf("hunks = %+v", files)
	}
	patch, err := HunkPatch(files[0], []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyPatch(ctx, patch, true, false); err != nil {
		t.Fatal(err)
	}
	staged, _ := r.Diff(ctx, DiffSpec{Path: r.abs("b.txt"), Staged: true})
	if !strings.Contains(staged, "+END changed") || strings.Contains(staged, "+BEE changed") {
		t.Fatalf("staged diff = %s", staged)
	}
	// Unstage it again via the reverse patch of the staged hunk.
	sf := ParseDiff(staged)
	up, _ := HunkPatch(sf[0], []int{0})
	if err := r.ApplyPatch(ctx, up, true, true); err != nil {
		t.Fatal(err)
	}
	if s, _ := r.Diff(ctx, DiffSpec{Path: r.abs("b.txt"), Staged: true}); s != "" {
		t.Fatalf("still staged: %s", s)
	}
	// Revert the first hunk in the work tree.
	wf := ParseDiff(text)
	rp, _ := HunkPatch(wf[0], []int{0})
	if err := r.ApplyPatch(ctx, rp, false, true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.abs("b.txt"))
	if !strings.HasPrefix(string(b), "BEE\n") || !strings.HasSuffix(string(b), "END changed\n") {
		t.Fatalf("after revert = %q", b)
	}
}

func TestStageUnstageDiscardCommitAmend(t *testing.T) {
	r, _ := newRepo(t)
	write(t, r.abs("a.txt"), "changed\n")
	write(t, r.abs("added.txt"), "x\n")
	if err := r.Stage(ctx, []string{r.abs("a.txt"), r.abs("added.txt")}); err != nil {
		t.Fatal(err)
	}
	st, _ := r.Status(ctx)
	if !find(st, "a.txt").Staged() || find(st, "added.txt").Index != "A" {
		t.Fatalf("after stage %+v", st.Files)
	}
	if err := r.Unstage(ctx, []string{r.abs("a.txt")}); err != nil {
		t.Fatal(err)
	}
	st, _ = r.Status(ctx)
	if find(st, "a.txt").Staged() {
		t.Fatal("unstage failed")
	}
	if err := r.Discard(ctx, []string{r.abs("a.txt"), r.abs("added.txt")}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(r.abs("a.txt")); !strings.HasPrefix(string(b), "one\n") {
		t.Fatalf("discard tracked = %q", b)
	}
	if _, err := os.Stat(r.abs("added.txt")); !os.IsNotExist(err) {
		t.Fatal("discarded added file still exists")
	}
	if err := r.Commit(ctx, "  ", false); err == nil {
		t.Fatal("empty message must fail")
	}
	write(t, r.abs("b.txt"), "b2\n")
	r.Stage(ctx, []string{r.abs("b.txt")})
	if err := r.Commit(ctx, "second\n\nbody text", false); err != nil {
		t.Fatal(err)
	}
	if m := r.LastMessage(ctx); m != "second\n\nbody text" {
		t.Fatalf("message = %q", m)
	}
	if err := r.Commit(ctx, "second (amended)", true); err != nil {
		t.Fatal(err)
	}
	log, _ := r.Log(ctx, LogQuery{})
	if len(log) != 2 || log[0].Subject != "second (amended)" || log[1].Subject != "initial" || len(log[0].Parents) != 1 {
		t.Fatalf("log = %+v", log)
	}
	files, _ := r.CommitFiles(ctx, log[0].Hash)
	if len(files) != 1 || files[0].Rel != "b.txt" || files[0].Status != "M" {
		t.Fatalf("commit files = %+v", files)
	}
	if c, _ := r.Show(ctx, "HEAD", r.abs("b.txt")); c != "b2\n" {
		t.Fatalf("show HEAD = %q", c)
	}
	if _, err := r.Show(ctx, "HEAD", r.abs("nope.txt")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("show missing = %v", err)
	}
}

func TestBranchesMergeConflictAndResolve(t *testing.T) {
	r, env := newRepo(t)
	if err := r.Checkout(ctx, "", "feature"); err != nil {
		t.Fatal(err)
	}
	write(t, r.abs("b.txt"), "feature bee\n")
	r.Stage(ctx, []string{r.abs("b.txt")})
	r.Commit(ctx, "feature change", false)
	r.Checkout(ctx, "main", "")
	write(t, r.abs("b.txt"), "main bee\n")
	r.Stage(ctx, []string{r.abs("b.txt")})
	r.Commit(ctx, "main change", false)

	bs, _ := r.Branches(ctx)
	names := map[string]bool{}
	for _, b := range bs {
		names[b.Name] = b.Current
	}
	if cur, ok := names["main"]; !ok || !cur || len(bs) != 2 {
		t.Fatalf("branches = %+v", bs)
	}
	if err := r.DeleteBranch(ctx, "feature", false); !errors.Is(err, ErrUnmerged) {
		t.Fatalf("delete unmerged = %v", err)
	}
	if err := r.Merge(ctx, "feature"); err == nil {
		t.Fatal("merge should conflict")
	}
	st, _ := r.Status(ctx)
	if st.Op != OpMerge {
		t.Fatalf("op = %q", st.Op)
	}
	f := find(st, "b.txt")
	if f == nil || !f.Conflict {
		t.Fatalf("conflict file = %+v", f)
	}
	ours, _ := r.Show(ctx, ":2", r.abs("b.txt"))
	theirs, _ := r.Show(ctx, ":3", r.abs("b.txt"))
	if ours != "main bee\n" || theirs != "feature bee\n" {
		t.Fatalf("stages: %q %q", ours, theirs)
	}
	text, _ := os.ReadFile(r.abs("b.txt"))
	blocks := Conflicts(string(text))
	if len(blocks) != 1 || blocks[0].TheirsLabel != "feature" {
		t.Fatalf("blocks = %+v in %q", blocks, text)
	}
	resolved := Resolve(string(text), blocks[0], "both")
	if resolved != "main bee\nfeature bee\n" {
		t.Fatalf("resolved = %q", resolved)
	}
	write(t, r.abs("b.txt"), resolved)
	r.Stage(ctx, []string{r.abs("b.txt")})
	if err := r.Continue(ctx); err != nil {
		t.Fatal(err)
	}
	if r.CurrentOp() != OpNone {
		t.Fatal("merge still in progress")
	}
	if err := r.DeleteBranch(ctx, "feature", false); err != nil {
		t.Fatalf("delete merged = %v", err)
	}
	_ = env
}

func TestAbortCherryPickAndRevert(t *testing.T) {
	r, _ := newRepo(t)
	r.Checkout(ctx, "", "other")
	write(t, r.abs("c.txt"), "cherry\n")
	r.Stage(ctx, []string{r.abs("c.txt")})
	r.Commit(ctx, "add c", false)
	log, _ := r.Log(ctx, LogQuery{Max: 1})
	r.Checkout(ctx, "main", "")
	if err := r.CherryPick(ctx, log[0].Hash); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(r.abs("c.txt")); string(b) != "cherry\n" {
		t.Fatal("cherry-pick did not apply")
	}
	head, _ := r.Log(ctx, LogQuery{Max: 1})
	if err := r.Revert(ctx, head[0].Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.abs("c.txt")); !os.IsNotExist(err) {
		t.Fatal("revert did not remove c.txt")
	}
	// Conflicting merge, then abort.
	r.Checkout(ctx, "", "x")
	write(t, r.abs("a.txt"), "x side\n")
	r.Stage(ctx, []string{r.abs("a.txt")})
	r.Commit(ctx, "x", false)
	r.Checkout(ctx, "main", "")
	write(t, r.abs("a.txt"), "main side\n")
	r.Stage(ctx, []string{r.abs("a.txt")})
	r.Commit(ctx, "m", false)
	if err := r.Rebase(ctx, "x"); err == nil {
		t.Fatal("rebase should conflict")
	}
	if r.CurrentOp() != OpRebase {
		t.Fatalf("op = %q", r.CurrentOp())
	}
	if err := r.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	if r.CurrentOp() != OpNone {
		t.Fatal("still rebasing")
	}
}

func TestStashBlameGitignoreRemoteCheckout(t *testing.T) {
	r, env := newRepo(t)
	write(t, r.abs("a.txt"), "stashed\n")
	write(t, r.abs("u.txt"), "untracked\n")
	if err := r.StashPush(ctx, "wip", true); err != nil {
		t.Fatal(err)
	}
	st, _ := r.Status(ctx)
	if len(st.Files) != 0 {
		t.Fatalf("after stash: %+v", st.Files)
	}
	list, _ := r.Stashes(ctx)
	if len(list) != 1 || !strings.Contains(list[0].Message, "wip") {
		t.Fatalf("stashes = %+v", list)
	}
	if d, err := r.StashDiff(ctx, 0); err != nil || !strings.Contains(d, "+stashed") {
		t.Fatalf("stash diff = %q %v", d, err)
	}
	if err := r.StashApply(ctx, 0, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(r.abs("u.txt")); string(b) != "untracked\n" {
		t.Fatal("pop lost the untracked file")
	}

	unsaved := "one\nNEW LINE\ntwo\n"
	bl, err := r.Blame(ctx, r.abs("a.txt"), &unsaved)
	if err != nil {
		t.Fatal(err)
	}
	if len(bl) != 3 || bl[0].Summary != "initial" || bl[0].Author != "Goi Test" || !strings.HasPrefix(bl[1].Hash, "0000000") || bl[0].Date == "" {
		t.Fatalf("blame = %+v", bl)
	}

	if err := r.AddToGitignore([]string{r.abs("u.txt"), r.abs("u.txt")}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(r.abs(".gitignore")); string(b) != "/u.txt\n" {
		t.Fatalf(".gitignore = %q", b)
	}

	// A bare remote: push sets the upstream; a remote branch checkout tracks.
	bare := t.TempDir()
	sh(t, bare, env, "init", "-q", "--bare")
	sh(t, r.Root, env, "remote", "add", "origin", bare)
	st, _ = r.Status(ctx)
	args := PushArgs(st, "", false)
	if strings.Join(args, " ") != "push --progress -u origin main" {
		t.Fatalf("push args = %v", args)
	}
	cmd := r.Command(ctx, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("push: %v %s", err, out)
	}
	sh(t, r.Root, env, "push", "-q", "origin", "main:topic")
	sh(t, r.Root, env, "fetch", "-q")
	if err := r.Checkout(ctx, "origin/topic", ""); err != nil {
		t.Fatal(err)
	}
	st, _ = r.Status(ctx)
	if st.Branch != "topic" || st.Upstream != "origin/topic" {
		t.Fatalf("tracking checkout: %+v", st)
	}
	if rs, _ := r.Remotes(ctx); len(rs) != 1 || rs[0] != "origin" {
		t.Fatalf("remotes = %v", rs)
	}
}

func TestParseHelpers(t *testing.T) {
	if !IsRejectedPush(" ! [rejected]        main -> main (fetch first)") || IsRejectedPush("Everything up-to-date") {
		t.Fatal("IsRejectedPush")
	}
	diff := "diff --git a/x.go b/x.go\nindex 1..2 100644\n--- a/x.go\n+++ b/x.go\n@@ -1,2 +1,2 @@ func x\n-a\n+b\n c\n"
	fd := ParseDiff(diff)
	if len(fd) != 1 || fd[0].NewPath != "x.go" || len(fd[0].Hunks) != 1 || fd[0].Hunks[0].OldStart != 1 || fd[0].Hunks[0].NewLines != 2 {
		t.Fatalf("ParseDiff = %+v", fd)
	}
	if _, err := HunkPatch(fd[0], nil); err == nil {
		t.Fatal("no hunks must fail")
	}
	if got := ParseDiff(""); len(got) != 0 {
		t.Fatal("empty diff")
	}
	diff3 := "a\n<<<<<<< HEAD\nours\n||||||| base\nbase\n=======\ntheirs\n>>>>>>> branch\nz\n<<<<<<< unterminated\n"
	b := Conflicts(diff3)
	if len(b) != 1 || b[0].Base != 3 {
		t.Fatalf("diff3 blocks = %+v", b)
	}
	if got := Resolve(diff3, b[0], "theirs"); !strings.HasPrefix(got, "a\ntheirs\nz\n") {
		t.Fatalf("resolve theirs = %q", got)
	}
}
