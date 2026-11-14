package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// ErrNoNpm means npm (Node.js) is not installed.
var ErrNoNpm = errors.New("npm not found: install Node.js (https://nodejs.org) to install agents")

// FindNpm locates npm on PATH.
func FindNpm(lookPath func(string) (string, error)) (string, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	p, err := lookPath("npm")
	if err != nil {
		return "", ErrNoNpm
	}
	return p, nil
}

// Install installs def's npm package into InstallDir(toolsDir, def.ID),
// streaming npm's output to out line by line.
func Install(ctx context.Context, def Definition, toolsDir, npm string, env []string, out func(line string)) error {
	if def.Package == "" {
		return fmt.Errorf("agent %s has no npm package", def.ID)
	}
	dir := InstallDir(toolsDir, def.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("install %s: %w", def.Name, err)
	}
	cmd := exec.CommandContext(ctx, npm, "install", "--prefix", dir, "--no-audit", "--no-fund", "--loglevel=error", def.Package)
	cmd.Dir = dir
	cmd.Env = env
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	var wg sync.WaitGroup
	var tail []string
	wg.Go(func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			if out != nil {
				out(line)
			}
			tail = append(tail, line)
			if len(tail) > 20 {
				tail = tail[1:]
			}
		}
		if sc.Err() != nil {
			_, _ = io.Copy(io.Discard, pr)
		}
	})
	err := cmd.Run()
	pw.Close()
	wg.Wait()
	if err != nil {
		return fmt.Errorf("install %s: npm install %s: %w\n%s", def.Name, def.Package, err, strings.Join(tail, "\n"))
	}
	return nil
}
