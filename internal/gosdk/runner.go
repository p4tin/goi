package gosdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Env is the project build environment applied to every go invocation.
type Env struct {
	Vars   map[string]string `json:"vars"`
	Tags   []string          `json:"tags"`
	GOOS   string            `json:"goos"`
	GOARCH string            `json:"goarch"`
	CGO    string            `json:"cgo"`    // CGO_ENABLED value: "", "0" or "1"
	Vendor string            `json:"vendor"` // auto | on | off
}

// Runner builds go commands for a project with its SDK and environment.
type Runner struct {
	SDK SDK
	Env Env
	Dir string
}

// buildFlagCommands are the go subcommands that accept -tags and -mod.
var buildFlagCommands = map[string]bool{
	"build": true, "test": true, "list": true, "vet": true,
	"run": true, "install": true, "generate": true,
}

// Command returns an exec.Cmd running the SDK's go binary with args, the
// project environment and build flags applied.
func (r Runner) Command(ctx context.Context, args ...string) *exec.Cmd {
	final := args
	if len(args) > 0 && buildFlagCommands[args[0]] {
		var flags []string
		if len(r.Env.Tags) > 0 {
			flags = append(flags, "-tags="+strings.Join(r.Env.Tags, ","))
		}
		switch r.Env.Vendor {
		case "on":
			flags = append(flags, "-mod=vendor")
		case "off":
			flags = append(flags, "-mod=mod")
		}
		final = append(append([]string{args[0]}, flags...), args[1:]...)
	}
	cmd := exec.CommandContext(ctx, r.SDK.GoBin, final...)
	cmd.Dir = r.Dir
	cmd.Env = r.environ()
	return cmd
}

func (r Runner) environ() []string {
	env := os.Environ()
	for k, v := range r.Env.Vars {
		env = setEnv(env, k, v)
	}
	if r.Env.GOOS != "" {
		env = setEnv(env, "GOOS", r.Env.GOOS)
	}
	if r.Env.GOARCH != "" {
		env = setEnv(env, "GOARCH", r.Env.GOARCH)
	}
	if r.Env.CGO != "" {
		env = setEnv(env, "CGO_ENABLED", r.Env.CGO)
	}
	if r.SDK.GOROOT != "" {
		env = setEnv(env, "GOROOT", r.SDK.GOROOT)
	}
	return env
}

// setEnv replaces or appends key=val. Keys compare case-insensitively on Windows.
func setEnv(env []string, key, val string) []string {
	for i, kv := range env {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if k == key || (runtime.GOOS == "windows" && strings.EqualFold(k, key)) {
			env[i] = key + "=" + val
			return env
		}
	}
	return append(env, key+"="+val)
}

// GoEnv runs `go env -json` with the project environment.
func (r Runner) GoEnv(ctx context.Context) (map[string]string, error) {
	out, err := r.Command(ctx, "env", "-json").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("gosdk: go env: %w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("gosdk: go env: %w", err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("gosdk: go env: decode: %w", err)
	}
	return m, nil
}

// IsCrossTarget reports whether the effective GOOS/GOARCH differ from the host.
func IsCrossTarget(goEnv map[string]string) bool {
	goos, goarch := goEnv["GOOS"], goEnv["GOARCH"]
	return (goos != "" && goos != runtime.GOOS) || (goarch != "" && goarch != runtime.GOARCH)
}
