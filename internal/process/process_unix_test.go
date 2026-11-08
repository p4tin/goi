//go:build !windows

package process

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestKillReachesGrandchildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	c := newCollector()
	p, err := Start(helper("parent", "GOI_PID_FILE="+pidFile), c.write)
	if err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, Stdout, "started")
	b, _ := os.ReadFile(pidFile)
	child, _ := strconv.Atoi(string(b))
	if child == 0 {
		t.Fatal("no grandchild pid")
	}
	p.Kill()
	p.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(child, 0) == nil {
		if time.Now().After(deadline) {
			syscall.Kill(child, syscall.SIGKILL)
			t.Fatal("grandchild survived the group kill")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
