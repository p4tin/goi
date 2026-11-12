package rpc

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/p4tin/goi/internal/events"
)

// isolateGit keeps the user's git config, hooks and signing out of a test.
func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// repoEvents returns the roots of the files in the git.status events of rec
// that report a repository.
func repoEvents(rec *events.Recorder) (n int, roots map[string]bool) {
	roots = map[string]bool{}
	for _, ev := range rec.Events(events.TopicGitStatus) {
		st := ev.Payload.(GitState)
		if !st.Repo {
			continue
		}
		n++
		for _, f := range st.Status.Files {
			roots[f.Path] = true
		}
	}
	return n, roots
}

func TestGitCallsActOnTheCallingWindow(t *testing.T) {
	isolateGit(t)
	app, w1, w2, r1, r2 := twoWindows(t)
	s1, _ := w1.current()
	s2, _ := w2.current()
	ctxA := withWindow(context.Background(), w1)
	ctxB := withWindow(context.Background(), w2)

	if app.gitSvc.State(ctxA).Repo || app.gitSvc.State(ctxB).Repo {
		t.Fatal("test modules should not be repositories yet")
	}
	// Init in A makes only A's project a repository.
	if err := app.gitSvc.Init(ctxA); err != nil {
		t.Fatal(err)
	}
	stA := app.gitSvc.State(ctxA)
	if !stA.Repo || len(stA.Status.Files) == 0 {
		t.Fatalf("A after Init = %+v", stA)
	}
	for _, f := range stA.Status.Files {
		if !within(s1.root, f.Path) {
			t.Errorf("A's status lists %s, outside %s", f.Path, s1.root)
		}
	}
	if app.gitSvc.State(ctxB).Repo {
		t.Fatal("Init in A made B a repository")
	}
	if _, err := app.gitSvc.Branches(ctxB); !errors.Is(err, errNoRepo) {
		t.Errorf("Branches in B: err = %v, want errNoRepo", err)
	}
	if n, _ := repoEvents(r1); n == 0 {
		t.Error("A got no git.status for its repository")
	}
	if n, _ := repoEvents(r2); n != 0 {
		t.Error("B got git.status for A's repository")
	}

	// A refresh in B reports B's repository to B only.
	if err := app.gitSvc.Init(ctxB); err != nil {
		t.Fatal(err)
	}
	app.gitSvc.Refresh(ctxB)
	_, rootsA := repoEvents(r1)
	_, rootsB := repoEvents(r2)
	if len(rootsB) == 0 {
		t.Fatal("B got no git.status files for its repository")
	}
	for p := range rootsA {
		if !within(s1.root, p) {
			t.Errorf("A's recorder got %s", p)
		}
	}
	for p := range rootsB {
		if !within(s2.root, p) {
			t.Errorf("B's recorder got %s", p)
		}
	}

	// With two windows, a call that names none has no window to act on.
	if _, err := app.gitSvc.Branches(context.Background()); !errors.Is(err, errNoWindow) {
		t.Errorf("Branches without a window: err = %v, want errNoWindow", err)
	}
}

func TestLintAndModCallsResolveTheCallingWindow(t *testing.T) {
	app, w1, w2, _, _ := twoWindows(t)
	ctxA := withWindow(context.Background(), w1)
	if err := w2.closeProject(); err != nil {
		t.Fatal(err)
	}
	ctxB := withWindow(context.Background(), w2)
	// B has no project: its calls fail even though A has one.
	if st := app.lintSvc.Status(ctxB); st.Message != errNoProject.Error() {
		t.Errorf("lint status in B = %+v, want no project", st)
	}
	if _, err := app.lintSvc.Run(ctxB, "project", ""); !errors.Is(err, errNoProject) {
		t.Errorf("lint run in B: err = %v, want errNoProject", err)
	}
	if _, err := app.modSvc.Versions(ctxB, "example.com/x"); !errors.Is(err, errNoProject) {
		t.Errorf("mod versions in B: err = %v, want errNoProject", err)
	}
	if st := app.lintSvc.Status(ctxA); st.Message == errNoProject.Error() {
		t.Errorf("lint status in A = %+v", st)
	}
}
