package rpc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/internal/settings"
)

// runModule is a module with an interactive main package and tests.
func runModule(t *testing.T) string {
	t.Helper()
	// module() creates an empty .git, which breaks go build's VCS stamping.
	t.Setenv("GOFLAGS", "-buildvcs=false")
	root := module(t)
	os.MkdirAll(filepath.Join(root, "cmd", "app"), 0o755)
	os.WriteFile(filepath.Join(root, "cmd", "app", "main.go"), []byte(`package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	fmt.Println("hello", os.Getenv("GREETING"), len(os.Args)-1)
	fmt.Fprintln(os.Stderr, "to stderr")
	sc := bufio.NewScanner(os.Stdin)
	if sc.Scan() {
		fmt.Println("read:", sc.Text())
	}
	os.Exit(4)
}
`), 0o644)
	os.MkdirAll(filepath.Join(root, "calc"), 0o755)
	os.WriteFile(filepath.Join(root, "calc", "calc.go"), []byte("package calc\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	os.WriteFile(filepath.Join(root, "calc", "calc_test.go"), []byte(`package calc

import "testing"

func TestAdd(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		if Add(1, 2) != 3 {
			t.Fatal("bad")
		}
	})
	t.Run("broken case", func(t *testing.T) {
		t.Fatal("expected failure")
	})
}

func TestSkip(t *testing.T) { t.Skip("later") }

func BenchmarkAdd(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Add(i, i)
	}
}
`), 0o644)
	return root
}

// waitRun waits for run id to report one of states in a run.state event
// after the first `since` events. Reaching another final state fails fast.
func waitRun(t *testing.T, bus *events.Recorder, id string, since int, states ...string) RunInfo {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		evs := bus.Events(events.TopicRunState)
		for _, e := range evs[min(since, len(evs)):] {
			info := e.Payload.(RunInfo)
			if info.ID != id {
				continue
			}
			for _, s := range states {
				if info.State == s {
					return info
				}
			}
			if info.State == RunExited || info.State == RunFailed {
				t.Fatalf("run %s ended %s (%s) while waiting for %v; output %+v", id, info.State, info.Message, states, runOutput(bus, id))
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s never reached %v; states: %+v", id, states, bus.Events(events.TopicRunState))
	return RunInfo{}
}

func runOutput(bus *events.Recorder, id string) map[string]string {
	out := map[string]string{}
	for _, e := range bus.Events(events.TopicRunOutput) {
		ev := e.Payload.(RunOutputEvent)
		if ev.ID != id {
			continue
		}
		for _, c := range ev.Chunks {
			out[c.Stream] += c.Text
		}
	}
	return out
}

func waitOutput(t *testing.T, bus *events.Recorder, id, stream, text string) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for !strings.Contains(runOutput(bus, id)[stream], text) {
		if time.Now().After(deadline) {
			t.Fatalf("no %q on %s; got %+v", text, stream, runOutput(bus, id))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunConfigCRUDAndEvents(t *testing.T) {
	root := runModule(t)
	a, bus := started(t, root)
	c := runconfig.Config{Name: "App", Type: runconfig.TypeApp, Kind: runconfig.KindPackage, Package: "./cmd/app", Shared: true}
	if _, err := a.runSvc.SaveConfig(context.Background(), c, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".goi", "runConfigurations", "App.toml")); err != nil {
		t.Fatal("shared configuration not written to .goi/runConfigurations")
	}
	if got := a.runSvc.Configs(context.Background()); len(got) != 1 || got[0].Name != "App" || !got[0].Shared || got[0].Files == nil {
		t.Fatalf("Configs = %+v", got)
	}
	if len(bus.Events(events.TopicRunConfigs)) == 0 {
		t.Fatal("no run.configs event")
	}
	if errs := a.runSvc.Validate(context.Background(), runconfig.Config{Name: "x", Type: runconfig.TypeApp}); len(errs) == 0 {
		t.Fatal("Validate should report a missing package")
	}
	// A configuration file added on disk is picked up by the watcher.
	os.WriteFile(filepath.Join(root, ".goi", "runConfigurations", "Disk.toml"), []byte("name = \"From disk\"\ntype = \"test\"\nkind = \"directory\"\ndirectory = \".\"\n"), 0o644)
	deadline := time.Now().Add(10 * time.Second)
	for len(a.runSvc.Configs(context.Background())) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("watcher did not reload: %+v", a.runSvc.Configs(context.Background()))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := a.runSvc.DeleteConfig(context.Background(), "App"); err != nil {
		t.Fatal(err)
	}
}

func TestRunAppWithInputAndExitCode(t *testing.T) {
	a, bus := started(t, runModule(t))
	c := runconfig.Config{Name: "App", Type: runconfig.TypeApp, Kind: runconfig.KindPackage, Package: "./cmd/app",
		Args: "one two", Env: map[string]string{"GREETING": "world"}}
	if _, err := a.runSvc.SaveConfig(context.Background(), c, ""); err != nil {
		t.Fatal(err)
	}
	id, err := a.runSvc.Run(context.Background(), "App", false)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, bus, id, 0, RunRunning)
	waitOutput(t, bus, id, "stdout", "hello world 2")
	if err := a.runSvc.Input(context.Background(), id, "typed\n"); err != nil {
		t.Fatal(err)
	}
	info := waitRun(t, bus, id, 0, RunExited, RunFailed)
	if info.State != RunExited || info.Exit == nil || info.Exit.Code != 4 {
		t.Fatalf("final = %+v", info)
	}
	out := runOutput(bus, id)
	if !strings.Contains(out["stdout"], "read: typed") || !strings.Contains(out["stderr"], "to stderr") || !strings.Contains(out[streamSystem], "build -o") {
		t.Fatalf("output = %+v", out)
	}
	if a.runSvc.Active(context.Background()) != 0 {
		t.Fatal("no run should be active")
	}
}

func TestRunTestsStreamsEvents(t *testing.T) {
	root := runModule(t)
	a, bus := started(t, root)
	cfg, err := a.runSvc.ContextConfig(context.Background(), ContextRequest{Path: filepath.Join(root, "calc"), Kind: "dir"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "go test ./calc" || !cfg.Temporary || cfg.Kind != runconfig.KindDirectory {
		t.Fatalf("context config = %+v", cfg)
	}
	id, err := a.runSvc.Run(context.Background(), cfg.Name, false)
	if err != nil {
		t.Fatal(err)
	}
	info := waitRun(t, bus, id, 0, RunExited, RunFailed)
	if !info.Test || info.Exit == nil || info.Exit.Code == 0 {
		t.Fatalf("final = %+v", info)
	}
	results := map[string]string{}
	for _, e := range bus.Events(events.TopicRunTest) {
		for _, ev := range e.Payload.(RunTestEvent).Events {
			if ev.Test != "" && (ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip") {
				results[ev.Test] = ev.Action
			}
		}
	}
	want := map[string]string{"TestAdd": "fail", "TestAdd/ok": "pass", "TestAdd/broken_case": "fail", "TestSkip": "skip"}
	for k, v := range want {
		if results[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, results[k], v, results)
		}
	}

	// Rerun only the failed subtest, in the same run.
	rerun := cfg
	rerun.Kind, rerun.Packages, rerun.Pattern, rerun.Directory = runconfig.KindFunction, []string{"example.com/m/calc"}, runconfig.TestRunPattern([]string{"TestAdd/broken_case"}), ""
	before := len(bus.Events(events.TopicRunTest))
	since := len(bus.Events(events.TopicRunState))
	if _, err := a.runSvc.RunConfig(context.Background(), rerun, id, false); err != nil {
		t.Fatal(err)
	}
	waitRun(t, bus, id, since, RunExited)
	ran := map[string]bool{}
	for _, e := range bus.Events(events.TopicRunTest)[before:] {
		for _, ev := range e.Payload.(RunTestEvent).Events {
			if ev.Action == "run" {
				ran[ev.Test] = true
			}
		}
	}
	if !ran["TestAdd/broken_case"] || ran["TestAdd/ok"] || ran["TestSkip"] {
		t.Fatalf("rerun ran %v", ran)
	}
}

func TestRunBeforeLaunchFailureAborts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX false command")
	}
	a, bus := started(t, runModule(t))
	c := runconfig.Config{Name: "App", Type: runconfig.TypeApp, Package: "./cmd/app",
		BeforeLaunch: []runconfig.Task{{Type: runconfig.TaskCommand, Command: "false"}}}
	a.runSvc.SaveConfig(context.Background(), c, "")
	id, err := a.runSvc.Run(context.Background(), "App", false)
	if err != nil {
		t.Fatal(err)
	}
	info := waitRun(t, bus, id, 0, RunFailed, RunExited)
	if info.State != RunFailed || !strings.Contains(info.Message, "Before launch 1") {
		t.Fatalf("final = %+v", info)
	}
	if strings.Contains(runOutput(bus, id)[streamSystem], "build -o") {
		t.Fatal("build must not run after a failed task")
	}
}

func TestRunStop(t *testing.T) {
	a, bus := started(t, runModule(t))
	a.runSvc.SaveConfig(context.Background(), runconfig.Config{Name: "App", Type: runconfig.TypeApp, Package: "./cmd/app"}, "")
	id, _ := a.runSvc.Run(context.Background(), "App", false)
	waitOutput(t, bus, id, "stdout", "hello")
	if a.runSvc.Active(context.Background()) != 1 {
		t.Fatalf("Active = %d", a.runSvc.Active(context.Background()))
	}
	if err := a.runSvc.Stop(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	info := waitRun(t, bus, id, 0, RunExited)
	if info.Exit == nil || (runtime.GOOS != "windows" && info.Exit.Signal == "" && info.Exit.Code == 4) {
		t.Fatalf("stopped run = %+v", info.Exit)
	}
	if err := a.runSvc.Close(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if len(a.runSvc.Runs(context.Background())) != 0 {
		t.Fatal("closed run still listed")
	}
}

func TestBenchHistoryRoundTrip(t *testing.T) {
	a, _ := started(t, runModule(t))
	if got, _ := a.runSvc.LoadBench(context.Background()); got != "" {
		t.Fatalf("empty history = %q", got)
	}
	if err := a.runSvc.SaveBench(context.Background(), `{"BenchmarkAdd":{"nsPerOp":1.5}}`); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.runSvc.LoadBench(context.Background()); !strings.Contains(got, "nsPerOp") {
		t.Fatalf("history = %q", got)
	}
	if a.runSvc.SaveBench(context.Background(), "{bad") == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}

func TestOutputBatcherKeepsRunesAndOrder(t *testing.T) {
	var got []OutputChunk
	b := newOutputBatcher(func(c []OutputChunk) { got = append(got, c...) })
	euro := []byte("€") // 3 bytes
	b.Write("stdout", []byte("a"))
	b.Write("stdout", euro[:1])
	b.Write("stderr", []byte("e"))
	b.Write("stdout", euro[1:])
	b.Close()
	text := ""
	for _, c := range got {
		text += c.Stream + ":" + c.Text + "|"
	}
	if text != "stdout:a|stderr:e|stdout:€|" {
		t.Fatalf("chunks = %s", text)
	}
}

func TestLineSplitter(t *testing.T) {
	var l lineSplitter
	if got := l.Write([]byte("a\r\nb")); len(got) != 1 || got[0] != "a" {
		t.Fatalf("lines = %q", got)
	}
	if got := l.Write([]byte("c\n")); len(got) != 1 || got[0] != "bc" {
		t.Fatalf("lines = %q", got)
	}
	l.Write([]byte("tail"))
	if l.Rest() != "tail" {
		t.Fatal("Rest")
	}
}

func TestParseTestLine(t *testing.T) {
	ev := parseTestLine(`{"Action":"pass","Package":"p","Test":"TestX","Elapsed":0.5}`)
	if ev.Action != "pass" || ev.Test != "TestX" || ev.Elapsed != 0.5 {
		t.Fatalf("ev = %+v", ev)
	}
	if ev := parseTestLine("# build error"); ev.Action != "output" || ev.Output != "# build error\n" {
		t.Fatalf("plain line = %+v", ev)
	}
}

func TestTerminalServiceRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	a, bus := started(t, runModule(t))
	a.allWindows()[0].settings.Set(settings.LayerUser, "terminal.shell", "/bin/sh")
	info, err := a.termSvc.Open(context.Background(), 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	if info.Shell != "/bin/sh" || a.termSvc.Count(context.Background()) != 1 {
		t.Fatalf("info = %+v", info)
	}
	a.termSvc.Write(context.Background(), info.ID, "echo term_$((1+1)); exit 7\n")
	deadline := time.Now().Add(15 * time.Second)
	for {
		out := ""
		for _, e := range bus.Events(events.TopicTermOutput) {
			out += e.Payload.(TermOutputEvent).Data
		}
		exits := bus.Events(events.TopicTermExit)
		if strings.Contains(out, "term_2") && len(exits) == 1 {
			if code := exits[0].Payload.(TermExitEvent).Code; code != 7 {
				t.Fatalf("exit code = %d", code)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("output %q exits %v", out, exits)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := a.termSvc.Close(context.Background(), info.ID); err != nil {
		t.Fatal(err)
	}
}
