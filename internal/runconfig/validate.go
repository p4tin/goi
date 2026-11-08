package runconfig

import (
	"fmt"
	"os"
	"strings"
)

// Validate returns human-readable problems with c; empty means runnable.
// Paths are checked relative to root.
func (c Config) Validate(root string) []string {
	ctx := Context{Root: root}
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
	case TypeApp, TypeTest:
		switch c.Kind {
		case KindPackage, "":
			if c.Type == TypeApp && strings.TrimSpace(c.Package) == "" && c.Directory == "" {
				add("Package is required")
			}
		case KindFile:
			if len(c.Files) == 0 {
				add("Choose at least one file")
			}
			for _, f := range c.Files {
				if !strings.HasSuffix(f, ".go") {
					add("%q is not a Go file", f)
				}
				exists("File", f, false)
			}
		case KindDirectory:
			if c.Directory == "" {
				add("Directory is required")
			}
			exists("Directory", c.Directory, true)
		case KindFunction:
			if c.Type != TypeTest {
				add("Function kind is only for tests")
			}
			if strings.TrimSpace(c.Pattern) == "" {
				add("Test pattern is required")
			}
		default:
			add("Unknown kind %q", c.Kind)
		}
		if c.Type == TypeTest && c.Fuzz != "" && c.Kind == KindDirectory {
			add("Fuzzing needs a single package")
		}
	case TypeRemote:
		if strings.TrimSpace(c.Host) == "" {
			add("Host is required")
		}
		if c.Port < 1 || c.Port > 65535 {
			add("Port must be between 1 and 65535")
		}
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
				dir = root
			}
			found := false
			for _, n := range []string{"GNUmakefile", "makefile", "Makefile"} {
				if _, err := os.Stat(dir + string(os.PathSeparator) + n); err == nil {
					found = true
				}
			}
			if !found {
				add("No Makefile in %s", dir)
			}
		}
	default:
		add("Unknown type %q", c.Type)
	}
	exists("Working directory", c.WorkingDir, true)
	exists("Environment file", c.EnvFile, false)
	exists("Input file", c.InputFile, false)
	if _, err := SplitArgs(c.Args); err != nil {
		add("Program arguments: %v", err)
	}
	if _, err := SplitArgs(c.BuildFlags); err != nil {
		add("Build flags: %v", err)
	}
	if _, err := SplitArgs(c.TestFlags); err != nil {
		add("Test flags: %v", err)
	}
	for i, t := range c.BeforeLaunch {
		switch t.Type {
		case TaskConfig:
			if strings.EqualFold(strings.TrimSpace(t.Config), c.Name) {
				add("Before launch task %d runs this configuration itself", i+1)
			} else if t.Config == "" {
				add("Before launch task %d: choose a configuration", i+1)
			}
		case TaskGenerate:
		case TaskCommand:
			if args, err := SplitArgs(t.Command); err != nil || len(args) == 0 {
				add("Before launch task %d: invalid command", i+1)
			}
		default:
			add("Before launch task %d: unknown type %q", i+1, t.Type)
		}
	}
	return errs
}
