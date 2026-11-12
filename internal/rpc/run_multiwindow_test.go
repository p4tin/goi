package rpc

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/internal/settings"
)

func TestRunCallsRouteByWindow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	a, w1, w2, r1, r2 := twoWindows(t)
	ctx1 := withWindow(context.Background(), w1)
	ctx2 := withWindow(context.Background(), w2)

	c := runconfig.Config{Name: "Hello", Type: runconfig.TypeShell, ScriptText: "echo hello_from_a"}
	before2 := len(r2.Events(events.TopicRunConfigs))
	if _, err := a.runSvc.SaveConfig(ctx1, c, ""); err != nil {
		t.Fatal(err)
	}
	if got := a.runSvc.Configs(ctx1); len(got) != 1 || got[0].Name != "Hello" {
		t.Fatalf("A configs = %+v", got)
	}
	if got := a.runSvc.Configs(ctx2); len(got) != 0 {
		t.Fatalf("B configs = %+v, want none", got)
	}
	if len(r2.Events(events.TopicRunConfigs)) != before2 {
		t.Fatal("B received A's run.configs event")
	}

	id, err := a.runSvc.Run(ctx1, "Hello", false)
	if err != nil {
		t.Fatal(err)
	}
	info := waitRun(t, r1, id, 0, RunExited, RunFailed)
	if info.State != RunExited || !strings.Contains(runOutput(r1, id)["stdout"], "hello_from_a") {
		t.Fatalf("A run = %+v, output %+v", info, runOutput(r1, id))
	}
	if n := len(r2.Events(events.TopicRunState)) + len(r2.Events(events.TopicRunOutput)); n != 0 {
		t.Fatalf("B received %d run events", n)
	}
	if len(a.runSvc.Runs(ctx1)) != 1 || len(a.runSvc.Runs(ctx2)) != 0 {
		t.Fatalf("runs: A %+v, B %+v", a.runSvc.Runs(ctx1), a.runSvc.Runs(ctx2))
	}
	if _, err := a.runSvc.Run(ctx2, "Hello", false); err == nil {
		t.Fatal("B must not see A's configuration")
	}
	// A context without a window fails when two windows exist.
	if _, err := a.runSvc.Run(context.Background(), "Hello", false); err == nil {
		t.Fatal("Run without a window must fail with two windows")
	}
}

func TestBreakpointsStayInTheirWindow(t *testing.T) {
	a, w1, w2, _, _ := twoWindows(t)
	ctx1 := withWindow(context.Background(), w1)
	if err := a.debugSvc.SetFileBreakpoints(ctx1, "/x/main.go", []LineBreakpoint{{Line: 3, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	s1, _ := w1.current()
	s2, _ := w2.current()
	s1.debug.mu.Lock()
	n1 := len(s1.debug.lines)
	s1.debug.mu.Unlock()
	s2.debug.mu.Lock()
	n2 := len(s2.debug.lines)
	s2.debug.mu.Unlock()
	if n1 != 1 || n2 != 0 {
		t.Fatalf("breakpoint files: A %d, B %d", n1, n2)
	}
}

func TestTerminalOutputStaysInItsWindow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	a, w1, w2, r1, r2 := twoWindows(t)
	w1.settings.Set(settings.LayerUser, "terminal.shell", "/bin/sh")
	ctx1 := withWindow(context.Background(), w1)
	ctx2 := withWindow(context.Background(), w2)
	info, err := a.termSvc.Open(ctx1, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if a.termSvc.Count(ctx1) != 1 || a.termSvc.Count(ctx2) != 0 {
		t.Fatalf("counts: A %d, B %d", a.termSvc.Count(ctx1), a.termSvc.Count(ctx2))
	}
	if err := a.termSvc.Write(ctx2, info.ID, "echo nope\n"); err == nil {
		t.Fatal("B must not write to A's terminal")
	}
	a.termSvc.Write(ctx1, info.ID, "echo win_$((2+3)); exit 0\n")
	deadline := time.Now().Add(15 * time.Second)
	for {
		out := ""
		for _, e := range r1.Events(events.TopicTermOutput) {
			out += e.Payload.(TermOutputEvent).Data
		}
		if strings.Contains(out, "win_5") && len(r1.Events(events.TopicTermExit)) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("A output %q", out)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(r2.Events(events.TopicTermOutput)) + len(r2.Events(events.TopicTermExit)); n != 0 {
		t.Fatalf("B received %d terminal events", n)
	}
}
