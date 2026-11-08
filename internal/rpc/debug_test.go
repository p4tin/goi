package rpc

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/toolbin"
)

func testDlv(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("GOI_DLV"); p != "" {
		return p
	}
	p, err := toolbin.Resolve(toolbin.Options{Name: "dlv", GOPATH: os.Getenv("GOPATH")})
	if err != nil {
		t.Skip("dlv not installed (set GOI_DLV)")
	}
	return p
}

// debugModule is a module with a goroutine program.
func debugModule(t *testing.T) string {
	t.Helper()
	root := runModule(t)
	os.MkdirAll(filepath.Join(root, "cmd", "work"), 0o755)
	os.WriteFile(filepath.Join(root, "cmd", "work", "main.go"), []byte(`package main

import (
	"fmt"
	"sync"
)

func work(i int, wg *sync.WaitGroup) {
	defer wg.Done()
	x := i * 2
	fmt.Println("work", x)
}

func main() {
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go work(i, &wg)
	}
	wg.Wait()
	fmt.Println("done")
}
`), 0o644)
	return root
}

func waitDebugState(t *testing.T, bus *events.Recorder, id, state string, since int) (DebugStateEvent, int) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		evs := bus.Events(events.TopicDebugState)
		for i := since; i < len(evs); i++ {
			ev := evs[i].Payload.(DebugStateEvent)
			if ev.ID == id && ev.State == state {
				return ev, i + 1
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no debug state %s; got %+v; run output %+v", state, bus.Events(events.TopicDebugState), runOutput(bus, id))
	return DebugStateEvent{}, 0
}

func TestDebugRequiresDlv(t *testing.T) {
	a, _ := started(t, runModule(t))
	if err := a.settings.Set(settings.LayerUser, "debug.dlvPath", filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatal(err)
	}
	a.runSvc.SaveConfig(runconfig.Config{Name: "App", Type: runconfig.TypeApp, Package: "./cmd/app"}, "")
	_, err := a.runSvc.Run("App", true)
	if err == nil || !strings.Contains(err.Error(), "dlv not found") {
		t.Fatalf("debug without dlv = %v", err)
	}
	if st := a.debugSvc.DlvStatus(); st.Found {
		t.Fatalf("DlvStatus = %+v", st)
	}
}

func TestDebugSessionBreakpointsAndInspection(t *testing.T) {
	dlv := testDlv(t)
	root := debugModule(t)
	a, bus := started(t, root)
	a.settings.Set(settings.LayerUser, "debug.dlvPath", dlv)
	if st := a.debugSvc.DlvStatus(); !st.Found || st.Path != dlv {
		t.Fatalf("DlvStatus = %+v", st)
	}
	src := filepath.Join(root, "cmd", "work", "main.go")
	// Conditional breakpoint, plus a disabled one that must not hit.
	if err := a.debugSvc.SetFileBreakpoints(src, []LineBreakpoint{{Line: 10, Condition: "i == 1", Enabled: true}, {Line: 22, Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	a.runSvc.SaveConfig(runconfig.Config{Name: "Work", Type: runconfig.TypeApp, Package: "./cmd/work"}, "")
	id, err := a.runSvc.Run("Work", true)
	if err != nil {
		t.Fatal(err)
	}
	st, next := waitDebugState(t, bus, id, DebugStopped, 0)
	if st.Reason != "breakpoint" || !st.Caps.SupportsSetVariable {
		t.Fatalf("stopped = %+v", st)
	}
	bps := bus.Events(events.TopicDebugBreakpoints)
	if len(bps) == 0 {
		t.Fatal("no breakpoint verification event")
	}
	threads, err := a.debugSvc.Threads(id)
	if err != nil || len(threads) < 2 {
		t.Fatalf("threads = %+v %v", threads, err)
	}
	frames, err := a.debugSvc.StackTrace(id, st.ThreadID, 0, 10)
	if err != nil || frames.StackFrames[0].Source == nil || frames.StackFrames[0].Source.Path != src || frames.StackFrames[0].Line != 10 {
		t.Fatalf("frames = %+v %v", frames, err)
	}
	r, err := a.debugSvc.Evaluate(id, "i", frames.StackFrames[0].ID, "watch")
	if err != nil || r.Result != "1" {
		t.Fatalf("i = %+v %v", r, err)
	}
	// Step over, then continue to the end.
	if err := a.debugSvc.Next(id, st.ThreadID); err != nil {
		t.Fatal(err)
	}
	st, next = waitDebugState(t, bus, id, DebugStopped, next)
	frames, _ = a.debugSvc.StackTrace(id, st.ThreadID, 0, 1)
	if frames.StackFrames[0].Line != 11 {
		t.Fatalf("after step line = %d", frames.StackFrames[0].Line)
	}
	if err := a.debugSvc.Continue(id, st.ThreadID); err != nil {
		t.Fatal(err)
	}
	waitDebugState(t, bus, id, DebugExited, next)
	info := waitRun(t, bus, id, 0, RunExited)
	if info.Exit == nil || info.Exit.Code != 0 || !info.Debug {
		t.Fatalf("run = %+v", info)
	}
	out := runOutput(bus, id)
	if !strings.Contains(out["stdout"], "done") {
		t.Fatalf("output = %+v", out)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "cmd", "work"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "__debug_bin") {
			t.Fatal("dlv left a debug binary in the project")
		}
	}
}

func TestDebugStopTerminates(t *testing.T) {
	dlv := testDlv(t)
	root := debugModule(t)
	a, bus := started(t, root)
	a.settings.Set(settings.LayerUser, "debug.dlvPath", dlv)
	a.debugSvc.SetFunctionBreakpoints([]FunctionBreakpoint{{Name: "main.work", Enabled: true}}, false)
	a.runSvc.SaveConfig(runconfig.Config{Name: "Work", Type: runconfig.TypeApp, Package: "./cmd/work"}, "")
	id, err := a.runSvc.Run("Work", true)
	if err != nil {
		t.Fatal(err)
	}
	waitDebugState(t, bus, id, DebugStopped, 0)
	if err := a.debugSvc.Stop(id); err != nil {
		t.Fatal(err)
	}
	waitRun(t, bus, id, 0, RunExited)
	if len(a.debugSvc.Sessions()) != 0 {
		t.Fatal("session still active after stop")
	}
}

func TestDebugRemoteAttach(t *testing.T) {
	dlv := testDlv(t)
	root := debugModule(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cmd := exec.Command(dlv, "debug", "--headless", "--listen=127.0.0.1:"+strconv.Itoa(port), "--api-version=2", "--accept-multiclient", "./cmd/work")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	ready := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.Contains(sc.Text(), "listening at") {
				close(ready)
				break
			}
		}
		for sc.Scan() {
		}
	}()
	select {
	case <-ready:
	case <-time.After(90 * time.Second):
		t.Fatal("headless dlv did not start")
	}

	a, bus := started(t, root)
	src := filepath.Join(root, "cmd", "work", "main.go")
	a.debugSvc.SetFileBreakpoints(src, []LineBreakpoint{{Line: 22, Enabled: true}})
	a.runSvc.SaveConfig(runconfig.Config{Name: "Remote", Type: runconfig.TypeRemote, Host: "127.0.0.1", Port: port}, "")
	if _, err := a.runSvc.Run("Remote", false); err == nil {
		t.Fatal("remote configurations can't be run")
	}
	id, err := a.runSvc.Run("Remote", true)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := waitDebugState(t, bus, id, DebugStopped, 0)
	if !st.Remote {
		t.Fatalf("stopped = %+v", st)
	}
	frames, _ := a.debugSvc.StackTrace(id, st.ThreadID, 0, 1)
	if frames.StackFrames[0].Line != 22 {
		t.Fatalf("remote stop line = %d", frames.StackFrames[0].Line)
	}
	a.debugSvc.Stop(id)
	waitRun(t, bus, id, 0, RunExited)
}
