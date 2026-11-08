// Package toolbin locates and installs Go developer tools (gopls, dlv) the
// same way for every tool: an explicit setting, then $PATH, $GOBIN, each
// $GOPATH/bin, then Goi's managed tools directory.
package toolbin

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

// ErrNotFound means no binary for the tool could be located.
var ErrNotFound = errors.New("not found")

// NotFoundError names the tool that was not found. It matches ErrNotFound.
type NotFoundError struct {
	Name   string
	Detail string
}

func (e *NotFoundError) Error() string {
	if e.Detail != "" {
		return e.Name + " not found: " + e.Detail
	}
	return e.Name + " not found"
}

// Is reports whether target is ErrNotFound.
func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }

// Options controls where Resolve looks.
type Options struct {
	// Name is the tool's base executable name without extension ("gopls").
	Name string
	// Setting is the user's explicit path; when set it is the only candidate.
	Setting string
	// LookPath searches $PATH (exec.LookPath when nil).
	LookPath func(file string) (string, error)
	// GOBIN and GOPATH come from `go env`.
	GOBIN, GOPATH string
	// ToolsDir is Goi's managed tools directory.
	ToolsDir string
}

// ExeName is name's executable file name on this OS.
func ExeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Resolve finds the tool: the setting, then $PATH, then $GOBIN, then each
// $GOPATH/bin, then the managed tools dir.
func Resolve(o Options) (string, error) {
	if o.Setting != "" {
		if IsExec(o.Setting) {
			return o.Setting, nil
		}
		return "", &NotFoundError{Name: o.Name, Detail: fmt.Sprintf("%q is not an executable", o.Setting)}
	}
	exe := ExeName(o.Name)
	look := o.LookPath
	if look == nil {
		look = exec.LookPath
	}
	if p, err := look(exe); err == nil && p != "" {
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
		p := filepath.Join(d, exe)
		if IsExec(p) {
			return p, nil
		}
	}
	return "", &NotFoundError{Name: o.Name}
}

// IsExec reports whether p is an executable regular file.
func IsExec(p string) bool {
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return false
	}
	return runtime.GOOS == "windows" || st.Mode()&0o111 != 0
}

// Install runs `go install pkg` with GOBIN set to toolsDir and returns the
// installed binary's path (toolsDir/name). goBin is the SDK's go binary; env
// is the project environment.
func Install(ctx context.Context, goBin string, env []string, toolsDir, pkg, name string) (string, error) {
	// go install requires an absolute GOBIN.
	if abs, err := filepath.Abs(toolsDir); err == nil {
		toolsDir = abs
	}
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return "", fmt.Errorf("%s: install: %w", name, err)
	}
	cmd := exec.CommandContext(ctx, goBin, "install", pkg)
	cmd.Dir = toolsDir // outside any module, so go.mod settings don't apply
	cmd.Env = append(WithoutKeys(env, "GOBIN", "GOFLAGS"), "GOBIN="+toolsDir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: go install failed: %w\n%s", name, err, Tail(out.String(), 2000))
	}
	p := filepath.Join(toolsDir, ExeName(name))
	if !IsExec(p) {
		return "", fmt.Errorf("%s: install finished but %s is missing", name, p)
	}
	return p, nil
}

// WithoutKeys returns env without the given variables.
func WithoutKeys(env []string, keys ...string) []string {
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

// Tail returns the last n bytes of s, prefixed with an ellipsis if cut.
func Tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
