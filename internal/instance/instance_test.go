package instance

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestAcquireReleaseAndHeld(t *testing.T) {
	dir := t.TempDir()
	var focused atomic.Int32
	l, err := Acquire(dir, "h1", func() { focused.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(LockPath(dir, "h1")); err != nil {
		t.Fatalf("lock file: %v", err)
	}
	// Same pid counts as not held by "another" process.
	if Held(dir, "h1") {
		t.Error("Held should ignore our own pid")
	}
	if err := Focus(dir, "h1"); err != nil {
		t.Fatal(err)
	}
	if focused.Load() != 1 {
		t.Errorf("focused = %d, want 1", focused.Load())
	}
	l.Release()
	l.Release() // idempotent
	if _, err := os.Stat(LockPath(dir, "h1")); !os.IsNotExist(err) {
		t.Errorf("lock not removed: %v", err)
	}
}

// fakeHolder writes a lock record pointing at a live listener owned by
// another (live) pid, simulating a second process.
func fakeHolder(t *testing.T, dir, hash string) (*Lock, int) {
	t.Helper()
	var focused atomic.Int32
	l, err := Acquire(dir, hash, func() { focused.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	// Rewrite the pid to the parent process, which is alive and not us.
	rec, _ := readRecord(l.path)
	os.WriteFile(l.path, []byte(fmt.Sprintf("%d %d %s\n", os.Getppid(), rec.port, rec.token)), 0o600)
	return l, rec.port
}

func TestSecondAcquireIsHeld(t *testing.T) {
	dir := t.TempDir()
	_, port := fakeHolder(t, dir, "h2")
	_, err := Acquire(dir, "h2", nil)
	var held *ErrHeld
	if !errors.As(err, &held) || held.Port != port || held.Pid != os.Getppid() {
		t.Fatalf("err = %v, want ErrHeld{port %d}", err, port)
	}
	if !Held(dir, "h2") {
		t.Error("Held = false")
	}
}

func TestStaleLockReplaced(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	dead := cmd.Process.Pid
	os.MkdirAll(dir+"/instances", 0o755)
	os.WriteFile(LockPath(dir, "h3"), []byte(fmt.Sprintf("%d 1 deadbeef\n", dead)), 0o600)
	l, err := Acquire(dir, "h3", nil)
	if err != nil {
		t.Fatalf("stale lock not replaced: %v", err)
	}
	defer l.Release()
	rec, _ := readRecord(LockPath(dir, "h3"))
	if rec.pid != os.Getpid() {
		t.Errorf("lock pid = %d", rec.pid)
	}
}

func TestLivePidWithoutListenerIsStale(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/instances", 0o755)
	// Parent pid is alive, but nothing listens on port 1.
	os.WriteFile(LockPath(dir, "h4"), []byte(fmt.Sprintf("%d 1 deadbeef\n", os.Getppid())), 0o600)
	l, err := Acquire(dir, "h4", nil)
	if err != nil {
		t.Fatalf("unanswered lock not replaced: %v", err)
	}
	l.Release()
}

func TestWrongTokenRejected(t *testing.T) {
	dir := t.TempDir()
	var focused atomic.Int32
	l, err := Acquire(dir, "h5", func() { focused.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	rec, _ := readRecord(l.path)
	reply, err := send(rec.port, "focus wrongtoken")
	if err != nil || reply != "denied" {
		t.Errorf("reply = %q, %v", reply, err)
	}
	if focused.Load() != 0 {
		t.Error("focus ran with wrong token")
	}
}

func TestLaunchCommand(t *testing.T) {
	tests := []struct {
		goos, exe string
		name      string
		args      []string
	}{
		{"darwin", "/Applications/Goi.app/Contents/MacOS/goi", "open", []string{"-n", "-a", "/Applications/Goi.app", "--args", "--project", "/p"}},
		{"darwin", "/usr/local/bin/goi", "/usr/local/bin/goi", []string{"--project", "/p"}},
		{"linux", "/opt/goi/goi", "/opt/goi/goi", []string{"--project", "/p"}},
		{"windows", `C:\Goi\goi.exe`, `C:\Goi\goi.exe`, []string{"--project", "/p"}},
	}
	for _, tt := range tests {
		name, args := launchCommand(tt.goos, tt.exe, "/p")
		if name != tt.name || !reflect.DeepEqual(args, tt.args) {
			t.Errorf("%s %s: got %s %v", tt.goos, tt.exe, name, args)
		}
	}
}
