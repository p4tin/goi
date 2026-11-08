package rpc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/git"
)

// gitModule is a module in a real git repository with one commit, isolated
// from the user's git config.
func gitModule(t *testing.T) (string, []string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Goi Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "goi@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Goi Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "goi@example.com")
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/g\n\ngo 1.26\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	run := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run(root, "init", "-q", "-b", "main")
	run(root, "add", ".")
	run(root, "commit", "-q", "-m", "initial")
	bare := t.TempDir()
	run(bare, "init", "-q", "--bare")
	run(root, "remote", "add", "origin", bare)
	return root, nil
}

func lastGitState(t *testing.T, bus *events.Recorder, pred func(GitState) bool) GitState {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		evs := bus.Events(events.TopicGitStatus)
		if len(evs) > 0 {
			if st := evs[len(evs)-1].Payload.(GitState); pred(st) {
				return st
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no matching git.status; events %+v", bus.Events(events.TopicGitStatus))
	return GitState{}
}

func TestGitNotRepo(t *testing.T) {
	a, bus := started(t, module(t))
	st := lastGitState(t, bus, func(s GitState) bool { return true })
	if st.Repo || st.Status.Files == nil {
		t.Fatalf("state = %+v", st)
	}
	if _, err := a.gitSvc.Branches(); err == nil {
		t.Fatal("Branches outside a repo should fail")
	}
}

func TestGitStatusStageCommitPush(t *testing.T) {
	root, _ := gitModule(t)
	a, bus := started(t, root)
	lastGitState(t, bus, func(s GitState) bool { return s.Repo && s.Status.Branch == "main" })
	// An edit made on disk shows up through the watcher.
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { println(1) }\n"), 0o644)
	st := lastGitState(t, bus, func(s GitState) bool { return len(s.Status.Files) == 1 })
	if f := st.Status.Files[0]; f.Path != filepath.Join(root, "main.go") || f.Work != "M" {
		t.Fatalf("file = %+v", f)
	}
	d, err := a.gitSvc.Diff(filepath.Join(root, "main.go"), false, "", "")
	if err != nil || len(d) != 1 || len(d[0].Hunks) != 1 {
		t.Fatalf("diff = %+v %v", d, err)
	}
	if err := a.gitSvc.Stage([]string{filepath.Join(root, "main.go")}); err != nil {
		t.Fatal(err)
	}
	if err := a.gitSvc.Commit("print", false); err != nil {
		t.Fatal(err)
	}
	st = lastGitState(t, bus, func(s GitState) bool { return len(s.Status.Files) == 0 })
	if st.Status.Upstream != "" {
		t.Fatalf("upstream before push = %q", st.Status.Upstream)
	}
	id, err := a.gitSvc.Push(false)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		var done *GitOpEvent
		for _, e := range bus.Events(events.TopicGitOp) {
			ev := e.Payload.(GitOpEvent)
			if ev.OpID == id && ev.State != GitOpRunning {
				done = &ev
			}
		}
		if done != nil {
			if done.State != GitOpDone {
				t.Fatalf("push failed: %+v", done)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("push did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	out := ""
	for _, e := range bus.Events(events.TopicGitOutput) {
		for _, c := range e.Payload.(GitOutputEvent).Chunks {
			out += c.Text
		}
	}
	if !strings.Contains(out, "git push --progress -u origin main") {
		t.Fatalf("output = %q", out)
	}
	st = lastGitState(t, bus, func(s GitState) bool { return s.Status.Upstream == "origin/main" })
	if st.Status.Ahead != 0 {
		t.Fatalf("ahead = %d", st.Status.Ahead)
	}
	if bs, _ := a.gitSvc.Branches(); len(bs) != 2 {
		t.Fatalf("branches = %+v", bs)
	}
	if log, _ := a.gitSvc.Log(gitLogQuery()); len(log) != 2 || log[0].Subject != "print" {
		t.Fatalf("log = %+v", log)
	}
	if sr, _ := a.gitSvc.Show("HEAD~1", filepath.Join(root, "main.go")); !sr.Found || strings.Contains(sr.Text, "println") {
		t.Fatalf("show = %+v", sr)
	}
	if sr, _ := a.gitSvc.Show("HEAD", filepath.Join(root, "nope.go")); sr.Found {
		t.Fatal("missing file found")
	}
}

func TestGitRollbackTrashesUntracked(t *testing.T) {
	root, _ := gitModule(t)
	a, bus := started(t, root)
	p := filepath.Join(root, "scratch.go")
	os.WriteFile(p, []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.go"), []byte("changed\n"), 0o644)
	lastGitState(t, bus, func(s GitState) bool { return len(s.Status.Files) == 2 })
	if err := a.gitSvc.Rollback([]string{p, filepath.Join(root, "main.go")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("untracked file not removed")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "main.go")); !strings.Contains(string(b), "func main") {
		t.Fatal("tracked file not restored")
	}
}

func gitLogQuery() git.LogQuery { return git.LogQuery{Max: 10} }
