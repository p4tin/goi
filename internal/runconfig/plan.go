package runconfig

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Context is what planning needs from the project. The app pack fills it
// (pack.Run.Context).
type Context struct {
	// Root is the project root.
	Root string
	// Tool is the toolchain's main program (Goi: the go binary; Pyi: the
	// project's python).
	Tool string
	// Env is the project environment (os env plus the toolchain's settings).
	Env []string
	// TempDir receives built binaries.
	TempDir string
	// Types are the pack's configuration types.
	Types Types
	// Extra is pack-specific planning data (Goi: build tags, -mod flag and
	// import path resolution).
	Extra any
}

// Cmd is a command to start.
type Cmd struct {
	Path string   `json:"path"`
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
	Env  []string `json:"-"`
	// Stdin is a file to redirect into the process ("" = interactive).
	Stdin string `json:"stdin"`
}

// String renders the command line for display.
func (c Cmd) String() string {
	parts := []string{quoteArg(c.Path)}
	for _, a := range c.Args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, " \t\"'\\$`*?[]{}()|&;<>") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

// Plan is what launching a configuration runs.
type Plan struct {
	// Build compiles the program first (app configurations).
	Build *Cmd `json:"build"`
	// Exec is the process to run; nil for build-only configurations.
	Exec *Cmd `json:"exec"`
	// Test reports that Exec streams test events (test2json lines: Goi's
	// `go test -json`).
	Test bool `json:"test"`
}

// Abs resolves p against the project root ("" stays "").
func (c Config) Abs(ctx Context, p string) string { return c.abs(ctx, p) }

func (c Config) abs(ctx Context, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(ctx.Root, p)
}

// Environ returns the process environment: ctx.Env, then the .env file,
// then the configuration's variables.
func (c Config) Environ(ctx Context) ([]string, error) {
	env := append([]string{}, ctx.Env...)
	if c.EnvFile != "" {
		m, err := LoadEnvFile(c.abs(ctx, c.EnvFile))
		if err != nil {
			return nil, err
		}
		env = mergeEnv(env, m)
	}
	return mergeEnv(env, c.Env), nil
}

func mergeEnv(env []string, m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = SetEnv(env, k, m[k])
	}
	return env
}

// SetEnv replaces or appends key=val (case-insensitive keys on Windows).
func SetEnv(env []string, key, val string) []string {
	for i, kv := range env {
		k, _, ok := strings.Cut(kv, "=")
		if ok && (k == key || (runtime.GOOS == "windows" && strings.EqualFold(k, key))) {
			env[i] = key + "=" + val
			return env
		}
	}
	return append(env, key+"="+val)
}

// WorkDir is the configuration's working directory, or def.
func (c Config) WorkDir(ctx Context, def string) string { return c.workDir(ctx, def) }

func (c Config) workDir(ctx Context, def string) string {
	if c.WorkingDir != "" {
		return c.abs(ctx, c.WorkingDir)
	}
	return def
}

// MakePlan turns the configuration into commands. Shell and Makefile are
// built in; other types go to their handler in ctx.Types.
func (c Config) MakePlan(ctx Context) (Plan, error) {
	env, err := c.Environ(ctx)
	if err != nil {
		return Plan{}, err
	}
	args, err := SplitArgs(c.Args)
	if err != nil {
		return Plan{}, fmt.Errorf("program arguments: %w", err)
	}
	switch c.Type {
	case TypeShell:
		dir := c.workDir(ctx, ctx.Root)
		interp := c.Interpreter
		var cmdArgs []string
		if interp == "" {
			if runtime.GOOS == "windows" {
				interp = "cmd.exe"
			} else {
				interp = "/bin/sh"
			}
		}
		ip, err := SplitArgs(interp)
		if err != nil || len(ip) == 0 {
			return Plan{}, errors.New("invalid interpreter")
		}
		cmdArgs = append(cmdArgs, ip[1:]...)
		if c.ScriptText != "" {
			if runtime.GOOS == "windows" && strings.EqualFold(filepath.Base(ip[0]), "cmd.exe") {
				cmdArgs = append(cmdArgs, "/C", c.ScriptText)
			} else {
				cmdArgs = append(cmdArgs, "-c", c.ScriptText)
			}
		} else {
			if runtime.GOOS == "windows" && strings.EqualFold(filepath.Base(ip[0]), "cmd.exe") {
				cmdArgs = append(cmdArgs, "/C")
			}
			cmdArgs = append(cmdArgs, c.abs(ctx, c.Script))
		}
		return Plan{Exec: &Cmd{Path: ip[0], Args: append(cmdArgs, args...), Dir: dir, Env: env}}, nil
	case TypeMake:
		dir := c.workDir(ctx, ctx.Root)
		var cmdArgs []string
		if c.Makefile != "" {
			mf := c.abs(ctx, c.Makefile)
			cmdArgs = append(cmdArgs, "-f", mf)
			if c.WorkingDir == "" {
				dir = filepath.Dir(mf)
			}
		}
		cmdArgs = append(append(cmdArgs, c.Targets...), args...)
		return Plan{Exec: &Cmd{Path: "make", Args: cmdArgs, Dir: dir, Env: env}}, nil
	}
	if h := ctx.Types[c.Type]; h != nil {
		return h.Plan(c, ctx)
	}
	return Plan{}, fmt.Errorf("unknown configuration type %q", c.Type)
}
