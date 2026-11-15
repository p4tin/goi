package gorun

import (
	"fmt"
	"os"
	"strings"

	"github.com/p4tin/goi/internal/runconfig"
)

// validate checks the Go fields of an app, test or remote configuration.
func validate(c runconfig.Config, ctx runconfig.Context) []string {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	exists := func(label, p string, wantDir bool) {
		if p == "" {
			return
		}
		st, err := os.Stat(c.Abs(ctx, p))
		switch {
		case err != nil:
			add("%s %q does not exist", label, p)
		case wantDir && !st.IsDir():
			add("%s %q is not a directory", label, p)
		case !wantDir && st.IsDir():
			add("%s %q is a directory", label, p)
		}
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
	}
	if _, err := runconfig.SplitArgs(c.BuildFlags); err != nil {
		add("Build flags: %v", err)
	}
	if _, err := runconfig.SplitArgs(c.TestFlags); err != nil {
		add("Test flags: %v", err)
	}
	return errs
}
