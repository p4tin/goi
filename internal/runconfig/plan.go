package runconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/p4tin/goi/internal/gosyntax"
)

// Context is what planning needs from the project.
type Context struct {
	// Root is the project root.
	Root string
	// GoBin is the SDK's go binary.
	GoBin string
	// Env is the project environment (os env + go.env + SDK settings).
	Env []string
	// Tags are the project's build tags (merged with the configuration's).
	Tags []string
	// ModFlag is "-mod=vendor", "-mod=mod" or "".
	ModFlag string
	// TempDir receives built binaries.
	TempDir string
	// PackageDir resolves an import path to its directory; nil means the
	// package is resolved relative to Root only.
	PackageDir func(importPath string) (string, error)
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
	// Test reports that Exec is `go test -json`.
	Test bool `json:"test"`
}

// Target is a resolved Go target.
type Target struct {
	// Dir is the package (or first file's) directory.
	Dir string
	// Arg is what to pass to go build / go test, relative to Dir.
	Arg []string
	// Recursive is set for directory-kind test targets (./...).
	Recursive bool
}

func (c Config) abs(ctx Context, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(ctx.Root, p)
}

// ResolveTarget resolves the configuration's Go target.
func (c Config) ResolveTarget(ctx Context) (Target, error) {
	switch c.Kind {
	case KindFile:
		if len(c.Files) == 0 {
			return Target{}, errors.New("no files selected")
		}
		files := make([]string, len(c.Files))
		for i, f := range c.Files {
			files[i] = c.abs(ctx, f)
		}
		dir := filepath.Dir(files[0])
		args := make([]string, len(files))
		for i, f := range files {
			if filepath.Dir(f) != dir {
				return Target{}, errors.New("all files must be in the same directory")
			}
			args[i] = filepath.Base(f)
		}
		return Target{Dir: dir, Arg: args}, nil
	case KindDirectory:
		d := c.abs(ctx, c.Directory)
		if d == "" {
			d = ctx.Root
		}
		return Target{Dir: d, Arg: []string{"."}, Recursive: c.Type == TypeTest}, nil
	default: // package, function
		pkg := strings.TrimSpace(c.Package)
		if pkg == "" && c.Directory != "" {
			return Target{Dir: c.abs(ctx, c.Directory), Arg: []string{"."}}, nil
		}
		if pkg == "" || pkg == "." {
			return Target{Dir: ctx.Root, Arg: []string{"."}}, nil
		}
		if strings.HasPrefix(pkg, ".") || filepath.IsAbs(pkg) {
			return Target{Dir: c.abs(ctx, filepath.FromSlash(pkg)), Arg: []string{"."}}, nil
		}
		if ctx.PackageDir != nil {
			if d, err := ctx.PackageDir(pkg); err == nil && d != "" {
				return Target{Dir: d, Arg: []string{"."}}, nil
			}
		}
		return Target{Dir: ctx.Root, Arg: []string{pkg}}, nil
	}
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

// buildFlags returns -tags/-mod and the configuration's extra build flags.
func (c Config) buildFlags(ctx Context) ([]string, error) {
	var out []string
	tags := mergeTags(ctx.Tags, c.Tags)
	if len(tags) > 0 {
		out = append(out, "-tags="+strings.Join(tags, ","))
	}
	if ctx.ModFlag != "" {
		out = append(out, ctx.ModFlag)
	}
	extra, err := SplitArgs(c.BuildFlags)
	if err != nil {
		return nil, fmt.Errorf("build flags: %w", err)
	}
	return append(out, extra...), nil
}

func mergeTags(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range append(append([]string{}, a...), b...) {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// BinaryPath is where an app configuration's binary is built.
func (c Config) BinaryPath(ctx Context) string {
	if c.Output != "" {
		return c.abs(ctx, c.Output)
	}
	name := "goi_" + Slug(c.Name)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(ctx.TempDir, name)
}

func (c Config) workDir(ctx Context, def string) string {
	if c.WorkingDir != "" {
		return c.abs(ctx, c.WorkingDir)
	}
	return def
}

// MakePlan turns the configuration into commands. It reads the .env file and,
// for file-kind tests, the test files.
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
	case TypeApp:
		t, err := c.ResolveTarget(ctx)
		if err != nil {
			return Plan{}, err
		}
		flags, err := c.buildFlags(ctx)
		if err != nil {
			return Plan{}, err
		}
		bin := c.BinaryPath(ctx)
		buildArgs := append(append([]string{"build", "-o", bin}, flags...), t.Arg...)
		p := Plan{Build: &Cmd{Path: ctx.GoBin, Args: buildArgs, Dir: t.Dir, Env: ctx.Env}}
		if c.RunsAfterBuild() {
			p.Exec = &Cmd{Path: bin, Args: args, Dir: c.workDir(ctx, t.Dir), Env: env, Stdin: c.abs(ctx, c.InputFile)}
		}
		return p, nil
	case TypeTest:
		t, err := c.ResolveTarget(ctx)
		if err != nil {
			return Plan{}, err
		}
		targs, err := c.TestArgs(ctx, t)
		if err != nil {
			return Plan{}, err
		}
		cmdArgs := append([]string{"test", "-json"}, targs...)
		if len(args) > 0 {
			cmdArgs = append(append(cmdArgs, "-args"), args...)
		}
		return Plan{Exec: &Cmd{Path: ctx.GoBin, Args: cmdArgs, Dir: c.workDir(ctx, t.Dir), Env: env}, Test: true}, nil
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
	case TypeRemote:
		return Plan{}, errors.New("Go Remote configurations can only be debugged")
	}
	return Plan{}, fmt.Errorf("unknown configuration type %q", c.Type)
}

// TestArgs returns the `go test` flags and package arguments for a test
// configuration (without -json).
func (c Config) TestArgs(ctx Context, t Target) ([]string, error) {
	flags, err := c.buildFlags(ctx)
	if err != nil {
		return nil, err
	}
	out := append([]string{}, flags...)
	if c.Race {
		out = append(out, "-race")
	}
	if c.Count > 0 {
		out = append(out, fmt.Sprintf("-count=%d", c.Count))
	}
	if c.Short {
		out = append(out, "-short")
	}
	if c.Timeout != "" {
		out = append(out, "-timeout="+c.Timeout)
	}
	run, err := c.runPattern(ctx)
	if err != nil {
		return nil, err
	}
	if run != "" {
		out = append(out, "-run="+run)
	}
	if c.Bench != "" {
		out = append(out, "-bench="+c.Bench)
		if c.Benchmem {
			out = append(out, "-benchmem")
		}
	}
	if c.Fuzz != "" {
		out = append(out, "-fuzz="+c.Fuzz)
		if c.FuzzTime != "" {
			out = append(out, "-fuzztime="+c.FuzzTime)
		}
	}
	extra, err := SplitArgs(c.TestFlags)
	if err != nil {
		return nil, fmt.Errorf("test flags: %w", err)
	}
	out = append(out, extra...)
	if t.Recursive {
		return append(out, "./..."), nil
	}
	if c.Kind == KindFile {
		return append(out, "."), nil
	}
	return append(out, t.Arg...), nil
}

// runPattern is the -run value: the pattern for function kind, the file's
// tests for file kind.
func (c Config) runPattern(ctx Context) (string, error) {
	switch c.Kind {
	case KindFunction:
		return c.Pattern, nil
	case KindFile:
		var names []string
		for _, f := range c.Files {
			p := c.abs(ctx, f)
			b, err := os.ReadFile(p)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", f, err)
			}
			for _, m := range gosyntax.RunMarkers(p, string(b)) {
				if m.Kind == gosyntax.KindTest || m.Kind == gosyntax.KindExample || m.Kind == gosyntax.KindFuzz {
					names = append(names, m.Name)
				}
			}
		}
		if len(names) == 0 {
			return "", errors.New("no tests in the selected files")
		}
		return TestRunPattern(names), nil
	}
	return c.Pattern, nil
}
