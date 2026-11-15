package runconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Validate returns human-readable problems with c; empty means runnable.
// Paths are checked relative to ctx.Root; pack types are checked by their
// handler in ctx.Types and pack tasks must have a handler in tasks.
func (c Config) Validate(ctx Context, tasks map[string]TaskHandler) []string {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	exists := func(label, p string, wantDir bool) {
		if p == "" {
			return
		}
		st, err := os.Stat(c.abs(ctx, p))
		switch {
		case err != nil:
			add("%s %q does not exist", label, p)
		case wantDir && !st.IsDir():
			add("%s %q is not a directory", label, p)
		case !wantDir && st.IsDir():
			add("%s %q is a directory", label, p)
		}
	}
	if strings.TrimSpace(c.Name) == "" {
		add("Name is required")
	}
	switch c.Type {
	case TypeShell:
		if c.Script == "" && strings.TrimSpace(c.ScriptText) == "" {
			add("Choose a script file or enter script text")
		}
		exists("Script", c.Script, false)
	case TypeMake:
		exists("Makefile", c.Makefile, false)
		if c.Makefile == "" {
			dir := c.abs(ctx, c.WorkingDir)
			if dir == "" {
				dir = ctx.Root
			}
			found := false
			for _, n := range []string{"GNUmakefile", "makefile", "Makefile"} {
				if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
					found = true
				}
			}
			if !found {
				add("No Makefile in %s", dir)
			}
		}
	default:
		if h := ctx.Types[c.Type]; h != nil {
			errs = append(errs, h.Validate(c, ctx)...)
		} else {
			add("Unknown type %q", c.Type)
		}
	}
	exists("Working directory", c.WorkingDir, true)
	exists("Environment file", c.EnvFile, false)
	exists("Input file", c.InputFile, false)
	if _, err := SplitArgs(c.Args); err != nil {
		add("Program arguments: %v", err)
	}
	for i, t := range c.BeforeLaunch {
		switch t.Type {
		case TaskConfig:
			if strings.EqualFold(strings.TrimSpace(t.Config), c.Name) {
				add("Before launch task %d runs this configuration itself", i+1)
			} else if t.Config == "" {
				add("Before launch task %d: choose a configuration", i+1)
			}
		case TaskCommand:
			if args, err := SplitArgs(t.Command); err != nil || len(args) == 0 {
				add("Before launch task %d: invalid command", i+1)
			}
		default:
			if tasks[t.Type] == nil {
				add("Before launch task %d: unknown type %q", i+1, t.Type)
			}
		}
	}
	return errs
}
