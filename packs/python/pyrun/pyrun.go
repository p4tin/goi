// Package pyrun is Pyi's run configuration type: Python ("python"), which
// runs a script with the project's interpreter, plus the gutter marker on
// `if __name__ == "__main__":` and "Run file" context runs.
package pyrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/packs/python/pyenv"
)

// TypePython runs a Python script.
const TypePython runconfig.Type = "python"

// Run is the pack.Run of Pyi.
type Run struct{}

var _ pack.Run = Run{}

// Types returns the Python type.
func (Run) Types() runconfig.Types { return runconfig.Types{TypePython: handler{}} }

// Tasks: Pyi adds no before-launch tasks.
func (Run) Tasks() map[string]runconfig.TaskHandler { return map[string]runconfig.TaskHandler{} }

// IsTest: Python runs aren't test streams (pytest is the python-app spec).
func (Run) IsTest(runconfig.Config) bool { return false }

// Context resolves the project interpreter and its environment.
func (Run) Context(_ context.Context, p pack.ProjectEnv, tempDir string) (runconfig.Context, error) {
	py, err := pyenv.Interpreter(p.Settings, p.Root)
	if err != nil {
		return runconfig.Context{}, fmt.Errorf("Python: %w", err)
	}
	return runconfig.Context{Root: p.Root, Tool: py, Env: pyenv.Environ(os.Environ(), py), TempDir: tempDir, Types: Run{}.Types()}, nil
}

// mainGuard matches `if __name__ == "__main__":` (either quote, either
// order).
var mainGuard = regexp.MustCompile(`^if\s+(__name__\s*==\s*['"]__main__['"]|['"]__main__['"]\s*==\s*__name__)\s*:`)

// Markers puts a run marker on a script's main guard.
func (Run) Markers(path, content string) []pack.RunMarker {
	if filepath.Ext(path) != ".py" {
		return nil
	}
	var out []pack.RunMarker
	for i, line := range strings.Split(content, "\n") {
		if mainGuard.MatchString(line) {
			out = append(out, pack.RunMarker{Kind: "main", Name: filepath.Base(path), Line: i + 1})
		}
	}
	return out
}

// ContextConfig runs the file (from its marker or the editor/tree).
func (Run) ContextConfig(req pack.ContextRequest, rel func(string) string) (runconfig.Config, error) {
	switch req.Kind {
	case "main", "file":
		if filepath.Ext(req.Path) != ".py" {
			return runconfig.Config{}, errors.New("not a Python file")
		}
		return runconfig.Config{Name: filepath.Base(req.Path), Type: TypePython, Script: rel(req.Path)}, nil
	}
	return runconfig.Config{}, fmt.Errorf("Pyi can't run a %q here", req.Kind)
}

type handler struct{}

// Plan runs `<interpreter> <script> <args>` in the working directory (the
// project root by default).
func (handler) Plan(c runconfig.Config, ctx runconfig.Context) (runconfig.Plan, error) {
	env, err := c.Environ(ctx)
	if err != nil {
		return runconfig.Plan{}, err
	}
	args, err := runconfig.SplitArgs(c.Args)
	if err != nil {
		return runconfig.Plan{}, fmt.Errorf("script arguments: %w", err)
	}
	py := ctx.Tool
	if c.Interpreter != "" {
		py = c.Abs(ctx, c.Interpreter)
		env = pyenv.Environ(env, py)
	}
	if py == "" {
		return runconfig.Plan{}, pyenv.ErrNoInterpreter
	}
	return runconfig.Plan{Exec: &runconfig.Cmd{
		Path:  py,
		Args:  append([]string{c.Abs(ctx, c.Script)}, args...),
		Dir:   c.WorkDir(ctx, ctx.Root),
		Env:   env,
		Stdin: c.Abs(ctx, c.InputFile),
	}}, nil
}

// Validate requires an existing .py script.
func (handler) Validate(c runconfig.Config, ctx runconfig.Context) []string {
	if strings.TrimSpace(c.Script) == "" {
		return []string{"Script is required"}
	}
	st, err := os.Stat(c.Abs(ctx, c.Script))
	if err != nil {
		return []string{fmt.Sprintf("Script %q does not exist", c.Script)}
	}
	if st.IsDir() {
		return []string{fmt.Sprintf("Script %q is a directory", c.Script)}
	}
	return nil
}

func (handler) Debuggable(runconfig.Config) bool { return false }
func (handler) Runnable(runconfig.Config) bool   { return true }
