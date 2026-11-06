package journal

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func TestFreshOpenNotCrashed(t *testing.T) {
	j, crashed, err := Open(t.TempDir(), "ws")
	if err != nil || crashed || j == nil {
		t.Fatalf("j=%v crashed=%v err=%v", j, crashed, err)
	}
}

func TestPutPendingDrop(t *testing.T) {
	j, _, err := Open(t.TempDir(), "ws")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Unix(100, 0).UTC()
	for _, p := range []string{"/b.go", "/a.go"} {
		if err := j.Put(Entry{Path: p, Content: "x" + p, BaseModTime: base}); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Put(Entry{Path: "/a.go", Content: "v2"}); err != nil { // replace
		t.Fatal(err)
	}
	got, err := j.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "/a.go" || got[0].Content != "v2" || got[1].Content != "x/b.go" {
		t.Fatalf("pending = %+v", got)
	}
	if got[0].SavedAt.IsZero() || !got[1].BaseModTime.Equal(base) {
		t.Errorf("times = %+v", got)
	}
	if err := j.Drop("/a.go"); err != nil {
		t.Fatal(err)
	}
	if err := j.Drop("/missing.go"); err != nil {
		t.Fatal(err)
	}
	got, _ = j.Pending()
	if len(got) != 1 || got[0].Path != "/b.go" {
		t.Fatalf("after drop = %+v", got)
	}
}

func TestCrashDetection(t *testing.T) {
	cache := t.TempDir()
	j, _, err := Open(cache, "ws")
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Put(Entry{Path: "/x.go", Content: "unsaved"}); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(cache, "journal", "ws", lockName)
	if err := os.WriteFile(lock, []byte(strconv.Itoa(deadPID(t))), 0o644); err != nil {
		t.Fatal(err)
	}

	j2, crashed, err := Open(cache, "ws")
	if err != nil {
		t.Fatal(err)
	}
	if !crashed {
		t.Error("expected crashed=true")
	}
	got, err := j2.Pending()
	if err != nil || len(got) != 1 || got[0].Content != "unsaved" {
		t.Fatalf("pending = %+v err = %v", got, err)
	}
	b, _ := os.ReadFile(lock)
	if string(b) != strconv.Itoa(os.Getpid()) {
		t.Errorf("lock = %q", b)
	}
}

func TestOwnPIDNotCrashed(t *testing.T) {
	cache := t.TempDir()
	if _, _, err := Open(cache, "ws"); err != nil {
		t.Fatal(err)
	}
	_, crashed, err := Open(cache, "ws")
	if err != nil || crashed {
		t.Fatalf("crashed=%v err=%v", crashed, err)
	}
}

func TestLiveOtherPIDInUse(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperSleep")
	cmd.Env = append(os.Environ(), "GOI_JOURNAL_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()
	cache := t.TempDir()
	dir := filepath.Join(cache, "journal", "ws")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, lockName), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
	if _, _, err := Open(cache, "ws"); !errors.Is(err, ErrInUse) {
		t.Fatalf("err = %v, want ErrInUse", err)
	}
}

func TestHelperSleep(t *testing.T) {
	if os.Getenv("GOI_JOURNAL_HELPER") != "1" {
		t.Skip("helper process")
	}
	time.Sleep(30 * time.Second)
}

func TestCloseClean(t *testing.T) {
	cache := t.TempDir()
	j, _, err := Open(cache, "ws")
	if err != nil {
		t.Fatal(err)
	}
	j.Put(Entry{Path: "/x.go", Content: "c"})
	if err := j.CloseClean(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(cache, "journal", "ws"))
	if len(entries) != 0 {
		t.Fatalf("leftovers: %v", entries)
	}
	_, crashed, err := Open(cache, "ws")
	if err != nil || crashed {
		t.Fatalf("crashed=%v err=%v", crashed, err)
	}
}
