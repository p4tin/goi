package runconfig

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DebugLaunch returns the dlv dap launch arguments for an app or test
// configuration. Program output goes to the client (outputMode remote) and
// the binary to bin, so Delve doesn't leave __debug_bin files in the
// project.
func (c Config) DebugLaunch(ctx Context, bin string) (map[string]any, error) {
	if c.Type != TypeApp && c.Type != TypeTest {
		return nil, fmt.Errorf("%s configurations can't be launched under the debugger", c.Type)
	}
	t, err := c.ResolveTarget(ctx)
	if err != nil {
		return nil, err
	}
	if t.Recursive || len(t.Arg) > 1 && c.Kind != KindFile {
		return nil, errors.New("debugging needs a single package: choose a package, file or function")
	}
	env, err := c.Environ(ctx)
	if err != nil {
		return nil, err
	}
	envMap := map[string]string{}
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			envMap[k] = v
		}
	}
	flags, err := c.buildFlags(ctx)
	if err != nil {
		return nil, err
	}
	args, err := SplitArgs(c.Args)
	if err != nil {
		return nil, fmt.Errorf("program arguments: %w", err)
	}
	program := t.Dir
	if c.Kind == KindFile && c.Type == TypeApp {
		if len(t.Arg) > 1 {
			return nil, errors.New("debugging a file configuration supports one file")
		}
		program = c.abs(ctx, c.Files[0])
	} else if len(t.Arg) == 1 && t.Arg[0] != "." {
		program = t.Arg[0]
	}
	launch := map[string]any{
		"request":     "launch",
		"mode":        "debug",
		"program":     program,
		"cwd":         c.workDir(ctx, t.Dir),
		"env":         envMap,
		"buildFlags":  joinFlags(flags),
		"output":      bin,
		"outputMode":  "remote",
		"stopOnEntry": false,
	}
	if c.Type == TypeTest {
		launch["mode"] = "test"
		var targs []string
		if c.Race {
			// -race is a build flag for test binaries.
			launch["buildFlags"] = joinFlags(append(flags, "-race"))
		}
		if c.Count > 0 {
			targs = append(targs, "-test.count="+strconv.Itoa(c.Count))
		}
		if c.Short {
			targs = append(targs, "-test.short")
		}
		if c.Timeout != "" {
			targs = append(targs, "-test.timeout="+c.Timeout)
		}
		run, err := c.runPattern(ctx)
		if err != nil {
			return nil, err
		}
		if run != "" {
			targs = append(targs, "-test.run="+run)
		}
		if c.Bench != "" {
			targs = append(targs, "-test.bench="+c.Bench)
			if c.Benchmem {
				targs = append(targs, "-test.benchmem")
			}
		}
		targs = append(targs, "-test.v")
		args = append(targs, args...)
	}
	launch["args"] = args
	return launch, nil
}

// joinFlags joins build flags into the single string dlv expects, quoting
// flags that contain spaces.
func joinFlags(flags []string) string {
	out := make([]string, len(flags))
	for i, f := range flags {
		if strings.ContainsAny(f, " \t") {
			out[i] = strconv.Quote(f)
		} else {
			out[i] = f
		}
	}
	return strings.Join(out, " ")
}
