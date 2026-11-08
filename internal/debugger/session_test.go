package debugger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/p4tin/goi/internal/dap"
	"github.com/p4tin/goi/internal/toolbin"
)

// dlvBin finds Delve for integration tests: $GOI_DLV, else the usual
// lookup. Tests skip when it is missing.
func dlvBin(t *testing.T) string {
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

const progSrc = `package main

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
	b := []byte("hello")
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go work(i, &wg)
	}
	wg.Wait()
	fmt.Println(string(b))
}
`

const testSrc = `package main

import "testing"

func TestWork(t *testing.T) {
	for _, n := range []int{1, 2, 3} {
		t.Run("case", func(t *testing.T) {
			v := n * 10
			_ = v
		})
	}
}
`

func sample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module sample\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(progSrc), 0o644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(testSrc), 0o644)
	return dir
}

type recorder struct {
	mu     sync.Mutex
	events []Event
	output strings.Builder
}

func (r *recorder) on(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	if ev.Event == "output" {
		var o dap.OutputEvent
		json.Unmarshal(ev.Body, &o)
		r.output.WriteString(o.Output)
	}
}

// wait returns the first event named name after index from.
func (r *recorder) wait(t *testing.T, name string, from int) (Event, int) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for i := from; i < len(r.events); i++ {
			if r.events[i].Event == name {
				ev := r.events[i]
				r.mu.Unlock()
				return ev, i + 1
			}
		}
		r.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no %q event; got %v", name, r.names())
	return Event{}, 0
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		out = append(out, e.Event)
	}
	return out
}

func start(t *testing.T, dir string, rec *recorder) *Session {
	t.Helper()
	ctx := context.Background()
	s, err := Start(ctx, Config{Dlv: dlvBin(t), Dir: dir, Env: append(os.Environ(), "GOFLAGS=-buildvcs=false"), OnEvent: rec.on})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if !s.Capabilities().SupportsConditionalBreakpoints {
		t.Fatalf("caps = %+v", s.Capabilities())
	}
	return s
}

func TestDebugProgramConditionalBreakpointAndInspect(t *testing.T) {
	dir := sample(t)
	rec := &recorder{}
	s := start(t, dir, rec)
	ctx := context.Background()
	bin := filepath.Join(t.TempDir(), "debug_bin")
	if err := s.Launch(ctx, map[string]any{"request": "launch", "mode": "debug", "program": dir, "output": bin, "outputMode": "remote"}); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.go")
	res, err := s.SetBreakpoints(ctx, main, []dap.SourceBreakpoint{{Line: 11, Condition: "i == 2"}, {Line: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || !res[0].Verified || res[1].Verified || res[1].Message == "" {
		t.Fatalf("breakpoints = %+v", res)
	}
	if err := s.ConfigurationDone(ctx); err != nil {
		t.Fatal(err)
	}
	ev, next := rec.wait(t, "stopped", 0)
	var st dap.StoppedEvent
	json.Unmarshal(ev.Body, &st)
	if st.Reason != "breakpoint" {
		t.Fatalf("stopped = %+v", st)
	}
	threads, err := s.Threads(ctx)
	if err != nil || len(threads) < 2 {
		t.Fatalf("threads = %+v %v", threads, err)
	}
	frames, err := s.StackTrace(ctx, st.ThreadID, 0, 20)
	if err != nil || frames.StackFrames[0].Line != 11 || frames.StackFrames[0].Name != "main.work" {
		t.Fatalf("frames = %+v %v", frames, err)
	}
	// Frames report the (possibly symlinked) path the breakpoint was set with.
	if p := frames.StackFrames[0].Source.Path; p != main {
		t.Fatalf("frame path = %q, want %q", p, main)
	}
	scopes, err := s.Scopes(ctx, frames.StackFrames[0].ID)
	if err != nil || len(scopes) == 0 {
		t.Fatalf("scopes = %+v %v", scopes, err)
	}
	vars, err := s.Variables(ctx, scopes[0].VariablesReference)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, v := range vars {
		got[v.Name] = v.Value
	}
	if got["i"] != "2" {
		t.Fatalf("conditional breakpoint stopped with i = %q (vars %+v)", got["i"], vars)
	}
	ev2, err := s.Evaluate(ctx, "i*10", frames.StackFrames[0].ID, "watch")
	if err != nil || ev2.Result != "20" {
		t.Fatalf("evaluate = %+v %v", ev2, err)
	}
	set, err := s.SetVariable(ctx, scopes[0].VariablesReference, "x", "99")
	if err != nil || set.Value != "99" {
		t.Fatalf("setVariable = %+v %v", set, err)
	}
	if err := s.Continue(ctx, st.ThreadID); err != nil {
		t.Fatal(err)
	}
	ex, _ := rec.wait(t, "exited", next)
	var exit dap.ExitedEvent
	json.Unmarshal(ex.Body, &exit)
	if exit.ExitCode != 0 {
		t.Fatalf("exit = %+v", exit)
	}
	rec.mu.Lock()
	out := rec.output.String()
	rec.mu.Unlock()
	if !strings.Contains(out, "work 99") || !strings.Contains(out, "hello") {
		t.Fatalf("program output = %q (set value should print 99)", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "__debug_bin")); err == nil {
		t.Fatal("dlv wrote a debug binary into the project")
	}
	s.Disconnect(ctx, true)
}

func TestDebugTestStepAndRunToCursor(t *testing.T) {
	dir := sample(t)
	rec := &recorder{}
	s := start(t, dir, rec)
	ctx := context.Background()
	bin := filepath.Join(t.TempDir(), "debug_test_bin")
	err := s.Launch(ctx, map[string]any{"mode": "test", "program": dir, "output": bin, "outputMode": "remote",
		"args": []string{"-test.run", "^TestWork$", "-test.v"}})
	if err != nil {
		t.Fatal(err)
	}
	test := filepath.Join(dir, "main_test.go")
	if _, err := s.SetBreakpoints(ctx, test, []dap.SourceBreakpoint{{Line: 8, HitCondition: "== 2"}}); err != nil {
		t.Fatal(err)
	}
	s.ConfigurationDone(ctx)
	ev, next := rec.wait(t, "stopped", 0)
	var st dap.StoppedEvent
	json.Unmarshal(ev.Body, &st)
	frames, _ := s.StackTrace(ctx, st.ThreadID, 0, 1)
	scopes, _ := s.Scopes(ctx, frames.StackFrames[0].ID)
	if r, err := s.Evaluate(ctx, "n", frames.StackFrames[0].ID, "watch"); err != nil || r.Result != "2" {
		t.Fatalf("hit condition: n = %+v %v (scopes %+v)", r, err, scopes)
	}
	if err := s.Next(ctx, st.ThreadID); err != nil {
		t.Fatal(err)
	}
	ev, next = rec.wait(t, "stopped", next)
	json.Unmarshal(ev.Body, &st)
	frames, _ = s.StackTrace(ctx, st.ThreadID, 0, 1)
	// "_ = v" on line 9 compiles to nothing, so the next stop is line 10.
	if frames.StackFrames[0].Line != 10 {
		t.Fatalf("after step over line = %d", frames.StackFrames[0].Line)
	}
	// Clear the user breakpoint; run to cursor on line 8 stops on the next
	// iteration (n == 3) and the temporary breakpoint goes away.
	s.SetBreakpoints(ctx, test, nil)
	if err := s.RunToCursor(ctx, st.ThreadID, test, 8); err != nil {
		t.Fatal(err)
	}
	ev, next = rec.wait(t, "stopped", next)
	json.Unmarshal(ev.Body, &st)
	frames, _ = s.StackTrace(ctx, st.ThreadID, 0, 1)
	if r, _ := s.Evaluate(ctx, "n", frames.StackFrames[0].ID, "watch"); r.Result != "3" {
		t.Fatalf("run to cursor stopped with n = %q", r.Result)
	}
	if err := s.RunToCursor(ctx, st.ThreadID, test, 1); err == nil {
		t.Fatal("run to a line without code should fail")
	}
	s.Continue(ctx, st.ThreadID)
	rec.wait(t, "terminated", next)
	rec.mu.Lock()
	out := rec.output.String()
	rec.mu.Unlock()
	if !strings.Contains(out, "PASS") {
		t.Fatalf("test output = %q", out)
	}
}

func TestStartFailsForBadProgram(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module bad\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() { undefined() }\n"), 0o644)
	rec := &recorder{}
	s := start(t, dir, rec)
	err := s.Launch(context.Background(), map[string]any{"mode": "debug", "program": dir, "output": filepath.Join(t.TempDir(), "b")})
	if err == nil || !strings.Contains(err.Error(), "undefined") {
		t.Fatalf("launch of a broken program = %v", err)
	}
}

func TestStartBadBinary(t *testing.T) {
	if _, err := Start(context.Background(), Config{Dlv: filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Fatal("want error for missing dlv")
	}
}
