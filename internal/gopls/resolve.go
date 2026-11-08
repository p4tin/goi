package gopls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrNotFound means no gopls binary could be located.
var ErrNotFound = errors.New("gopls not found")

// Module is the gopls module path installed by Install.
const Module = "golang.org/x/tools/gopls@latest"

// ResolveOptions controls where Resolve looks for gopls.
type ResolveOptions struct {
	// Setting is the gopls.path setting; when set it is the only candidate.
	Setting string
	// LookPath searches $PATH (exec.LookPath when nil).
	LookPath func(file string) (string, error)
	// GOBIN and GOPATH come from `go env`.
	GOBIN, GOPATH string
	// ToolsDir is Goi's managed tools directory.
	ToolsDir string
}

// ExeName is the gopls executable name on this OS.
func ExeName() string {
	if runtime.GOOS == "windows" {
		return "gopls.exe"
	}
	return "gopls"
}

// Resolve finds gopls: the gopls.path setting, then $PATH, then $GOBIN,
// then each $GOPATH/bin, then the managed tools dir.
func Resolve(o ResolveOptions) (string, error) {
	if o.Setting != "" {
		if isExec(o.Setting) {
			return o.Setting, nil
		}
		return "", fmt.Errorf("%w: gopls.path %q is not an executable", ErrNotFound, o.Setting)
	}
	look := o.LookPath
	if look == nil {
		look = exec.LookPath
	}
	if p, err := look(ExeName()); err == nil && p != "" {
		return p, nil
	}
	var dirs []string
	if o.GOBIN != "" {
		dirs = append(dirs, o.GOBIN)
	}
	for _, gp := range filepath.SplitList(o.GOPATH) {
		if gp != "" {
			dirs = append(dirs, filepath.Join(gp, "bin"))
		}
	}
	if o.ToolsDir != "" {
		dirs = append(dirs, o.ToolsDir)
	}
	for _, d := range dirs {
		p := filepath.Join(d, ExeName())
		if isExec(p) {
			return p, nil
		}
	}
	return "", ErrNotFound
}

func isExec(p string) bool {
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return false
	}
	return runtime.GOOS == "windows" || st.Mode()&0o111 != 0
}

// Install runs `go install golang.org/x/tools/gopls@latest` with GOBIN set
// to toolsDir. goBin is the SDK's go binary; env is the project
// environment. It returns the installed binary's path.
func Install(ctx context.Context, goBin string, env []string, toolsDir string) (string, error) {
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return "", fmt.Errorf("gopls: install: %w", err)
	}
	cmd := exec.CommandContext(ctx, goBin, "install", Module)
	cmd.Dir = toolsDir // outside any module, so go.mod settings don't apply
	cmd.Env = append(withoutKeys(env, "GOBIN", "GOFLAGS"), "GOBIN="+toolsDir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gopls: go install failed: %w\n%s", err, tail(out.String(), 2000))
	}
	p := filepath.Join(toolsDir, ExeName())
	if !isExec(p) {
		return "", fmt.Errorf("gopls: install finished but %s is missing", p)
	}
	return p, nil
}

func withoutKeys(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
outer:
	for _, kv := range env {
		for _, k := range keys {
			if strings.HasPrefix(kv, k+"=") {
				continue outer
			}
		}
		out = append(out, kv)
	}
	return out
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
