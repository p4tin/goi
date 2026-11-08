package process

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The test binary doubles as the helper program: GOI_PROC_HELPER selects a
// behavior.
func TestMain(m *testing.M) {
	switch os.Getenv("GOI_PROC_HELPER") {
	case "":
		os.Exit(m.Run())
	case "echo":
		fmt.Fprintln(os.Stderr, "ready")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fmt.Println("got:", sc.Text())
		}
		os.Exit(3)
	case "trap":
		// Exits cleanly on SIGINT.
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		fmt.Println("waiting")
		<-c
		fmt.Println("interrupted")
		os.Exit(0)
	case "ignore":
		signal.Ignore(os.Interrupt)
		fmt.Println("waiting")
		time.Sleep(time.Minute)
	case "parent":
		// Starts a grandchild that writes its pid, then waits forever.
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "GOI_PROC_HELPER=sleep")
		child.Start()
		os.WriteFile(os.Getenv("GOI_PID_FILE"), []byte(strconv.Itoa(child.Process.Pid)), 0o644)
		fmt.Println("started")
		time.Sleep(time.Minute)
	case "sleep":
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

type collector struct {
	mu  sync.Mutex
	out map[string]*strings.Builder
}

func newCollector() *collector {
	return &collector{out: map[string]*strings.Builder{Stdout: {}, Stderr: {}}}
}

func (c *collector) write(stream string, b []byte) {
	c.mu.Lock()
	c.out[stream].Write(b)
	c.mu.Unlock()
}

func (c *collector) get(stream string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out[stream].String()
}

func (c *collector) waitFor(t *testing.T, stream, text string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(c.get(stream), text) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q on %s; got %q", text, stream, c.get(stream))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func helper(mode string, extra ...string) Spec {
	return Spec{Path: os.Args[0], Env: append(append(os.Environ(), "GOI_PROC_HELPER="+mode), extra...)}
}

func TestStdinAndExitCode(t *testing.T) {
	c := newCollector()
	p, err := Start(helper("echo"), c.write)
	if err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, Stderr, "ready")
	if err := p.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, Stdout, "got: hello")
	p.CloseStdin()
	if info := p.Wait(); info.Code != 3 || info.Err != "" {
		t.Fatalf("exit = %+v", info)
	}
}

func TestStdinFromFile(t *testing.T) {
	in := filepath.Join(t.TempDir(), "in.txt")
	os.WriteFile(in, []byte("a\nb\n"), 0o644)
	spec := helper("echo")
	spec.StdinFile = in
	c := newCollector()
	p, err := Start(spec, c.write)
	if err != nil {
		t.Fatal(err)
	}
	p.Wait()
	if got := c.get(Stdout); got != "got: a\ngot: b\n" {
		t.Fatalf("stdout = %q", got)
	}
	if p.Write([]byte("x")) == nil {
		t.Fatal("Write must fail when stdin is a file")
	}
}

func TestStopInterruptsFirst(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows stop kills the tree")
	}
	c := newCollector()
	p, err := Start(helper("trap"), c.write)
	if err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, Stdout, "waiting")
	p.Stop()
	info := p.Wait()
	if info.Code != 0 || !strings.Contains(c.get(Stdout), "interrupted") {
		t.Fatalf("exit = %+v out = %q", info, c.get(Stdout))
	}
}

func TestStopEscalatesToKill(t *testing.T) {
	spec := helper("ignore")
	spec.KillAfter = 200 * time.Millisecond
	c := newCollector()
	p, err := Start(spec, c.write)
	if err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, Stdout, "waiting")
	start := time.Now()
	p.Stop()
	info := p.Wait()
	if runtime.GOOS != "windows" && info.Signal != "killed" {
		t.Fatalf("want killed, got %+v", info)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("kill escalation took too long")
	}
}

func TestSecondStopKills(t *testing.T) {
	c := newCollector()
	p, err := Start(helper("ignore"), c.write)
	if err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, Stdout, "waiting")
	p.Stop()
	p.Stop()
	select {
	case <-p.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("second Stop did not kill")
	}
	if !p.Stopping() {
		t.Fatal("Stopping should be true")
	}
}

func TestStartErrors(t *testing.T) {
	if _, err := Start(Spec{}, func(string, []byte) {}); err == nil {
		t.Fatal("empty spec must fail")
	}
	if _, err := Start(Spec{Path: filepath.Join(t.TempDir(), "missing")}, func(string, []byte) {}); err == nil {
		t.Fatal("missing binary must fail")
	}
	if _, err := Start(Spec{Path: os.Args[0], StdinFile: "/nonexistent/in"}, func(string, []byte) {}); err == nil {
		t.Fatal("missing input file must fail")
	}
}
