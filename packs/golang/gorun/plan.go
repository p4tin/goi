package gorun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/packs/golang/gosyntax"
)

// Target is a resolved Go target.
type Target struct {
	// Dir is the package (or first file's) directory.
	Dir string
	// Arg is what to pass to go build / go test, relative to Dir.
	Arg []string
	// Recursive is set for directory-kind test targets (./...).
	Recursive bool
}

// ResolveTarget resolves the configuration's Go target.
func ResolveTarget(c runconfig.Config, ctx runconfig.Context) (Target, error) {
	switch c.Kind {
	case KindFile:
		if len(c.Files) == 0 {
			return Target{}, errors.New("no files selected")
		}
		files := make([]string, len(c.Files))
		for i, f := range c.Files {
			files[i] = c.Abs(ctx, f)
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
		d := c.Abs(ctx, c.Directory)
		if d == "" {
			d = ctx.Root
		}
		return Target{Dir: d, Arg: []string{"."}, Recursive: c.Type == TypeTest}, nil
	default: // package, function
		if len(c.Packages) > 0 {
			return Target{Dir: ctx.Root, Arg: append([]string{}, c.Packages...)}, nil
		}
		pkg := strings.TrimSpace(c.Package)
		if pkg == "" && c.Directory != "" {
			return Target{Dir: c.Abs(ctx, c.Directory), Arg: []string{"."}}, nil
		}
		if pkg == "" || pkg == "." {
			return Target{Dir: ctx.Root, Arg: []string{"."}}, nil
		}
		if strings.HasPrefix(pkg, ".") || filepath.IsAbs(pkg) {
			return Target{Dir: c.Abs(ctx, filepath.FromSlash(pkg)), Arg: []string{"."}}, nil
		}
		if goContext(ctx).PackageDir != nil {
			if d, err := goContext(ctx).PackageDir(pkg); err == nil && d != "" {
				return Target{Dir: d, Arg: []string{"."}}, nil
			}
		}
		return Target{Dir: ctx.Root, Arg: []string{pkg}}, nil
	}
}

// buildFlags returns -tags/-mod and the configuration's extra build flags.
func buildFlags(c runconfig.Config, ctx runconfig.Context) ([]string, error) {
	var out []string
	tags := mergeTags(goContext(ctx).Tags, c.Tags)
	if len(tags) > 0 {
		out = append(out, "-tags="+strings.Join(tags, ","))
	}
	if goContext(ctx).ModFlag != "" {
		out = append(out, goContext(ctx).ModFlag)
	}
	extra, err := runconfig.SplitArgs(c.BuildFlags)
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
func BinaryPath(c runconfig.Config, ctx runconfig.Context) string {
	if c.Output != "" {
		return c.Abs(ctx, c.Output)
	}
	name := "goi_" + runconfig.Slug(c.Name)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(ctx.TempDir, name)
}

// TestArgs returns the `go test` flags and package arguments for a test
// configuration (without -json).
func TestArgs(c runconfig.Config, ctx runconfig.Context, t Target) ([]string, error) {
	flags, err := buildFlags(c, ctx)
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
	run, err := runPattern(c, ctx)
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
	extra, err := runconfig.SplitArgs(c.TestFlags)
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
func runPattern(c runconfig.Config, ctx runconfig.Context) (string, error) {
	switch c.Kind {
	case KindFunction:
		return c.Pattern, nil
	case KindFile:
		var names []string
		for _, f := range c.Files {
			p := c.Abs(ctx, f)
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

// TestRunPattern builds a `go test -run` pattern selecting the given tests,
// whose names are as reported by test2json ("TestA", "TestA/sub_case").
// Each level becomes an anchored alternation of the quoted names at that
// level, joined by "/". Selecting by level can include extra combinations
// (TestA/x and TestB/y also select TestA/y), which go test tolerates; every
// requested test is included.
func TestRunPattern(names []string) string {
	var levels [][]string
	seen := []map[string]bool{}
	for _, n := range names {
		for i, part := range strings.Split(n, "/") {
			if i == len(levels) {
				levels = append(levels, nil)
				seen = append(seen, map[string]bool{})
			}
			if !seen[i][part] {
				seen[i][part] = true
				levels[i] = append(levels[i], regexp.QuoteMeta(part))
			}
		}
	}
	// A top-level test listed without subtests must still run fully: drop
	// deeper levels if any name stops early.
	minDepth := -1
	for _, n := range names {
		d := len(strings.Split(n, "/"))
		if minDepth < 0 || d < minDepth {
			minDepth = d
		}
	}
	parts := make([]string, 0, len(levels))
	for i, l := range levels {
		if minDepth >= 0 && i >= minDepth {
			break
		}
		if len(l) == 1 {
			parts = append(parts, "^"+l[0]+"$")
		} else {
			parts = append(parts, "^("+strings.Join(l, "|")+")$")
		}
	}
	return strings.Join(parts, "/")
}

// planApp builds the binary, then runs it (unless RunAfterBuild is off).
func planApp(c runconfig.Config, ctx runconfig.Context, env, args []string) (runconfig.Plan, error) {
	t, err := ResolveTarget(c, ctx)
	if err != nil {
		return runconfig.Plan{}, err
	}
	flags, err := buildFlags(c, ctx)
	if err != nil {
		return runconfig.Plan{}, err
	}
	bin := BinaryPath(c, ctx)
	buildArgs := append(append([]string{"build", "-o", bin}, flags...), t.Arg...)
	p := runconfig.Plan{Build: &runconfig.Cmd{Path: ctx.Tool, Args: buildArgs, Dir: t.Dir, Env: ctx.Env}}
	if c.RunsAfterBuild() {
		p.Exec = &runconfig.Cmd{Path: bin, Args: args, Dir: c.WorkDir(ctx, t.Dir), Env: env, Stdin: c.Abs(ctx, c.InputFile)}
	}
	return p, nil
}

// planTest runs `go test -json`.
func planTest(c runconfig.Config, ctx runconfig.Context, env, args []string) (runconfig.Plan, error) {
	t, err := ResolveTarget(c, ctx)
	if err != nil {
		return runconfig.Plan{}, err
	}
	targs, err := TestArgs(c, ctx, t)
	if err != nil {
		return runconfig.Plan{}, err
	}
	cmdArgs := append([]string{"test", "-json"}, targs...)
	if len(args) > 0 {
		cmdArgs = append(append(cmdArgs, "-args"), args...)
	}
	return runconfig.Plan{Exec: &runconfig.Cmd{Path: ctx.Tool, Args: cmdArgs, Dir: c.WorkDir(ctx, t.Dir), Env: env}, Test: true}, nil
}
