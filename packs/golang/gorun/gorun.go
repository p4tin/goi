// Package gorun is Goi's run configuration types: Go Build ("app"), Go Test
// ("test") and Go Remote ("remote"), the `go generate` before-launch task,
// gutter run markers and the debugger launch arguments for Delve.
package gorun

import (
	"errors"
	"fmt"

	"github.com/p4tin/goi/internal/runconfig"
)

// Go configuration types.
const (
	TypeApp    runconfig.Type = "app"
	TypeTest   runconfig.Type = "test"
	TypeRemote runconfig.Type = "remote"
)

// Kinds of Go targets.
const (
	KindPackage   = "package"
	KindFile      = "file"
	KindDirectory = "directory"
	KindFunction  = "function"
)

// TaskGenerate is the `go generate` before-launch task.
const TaskGenerate = "generate"

// GoContext is the Go part of a planning context (runconfig.Context.Extra).
type GoContext struct {
	// Tags are the project's build tags (merged with the configuration's).
	Tags []string
	// ModFlag is "-mod=vendor", "-mod=mod" or "".
	ModFlag string
	// PackageDir resolves an import path to its directory; nil means the
	// package is resolved relative to Root only.
	PackageDir func(importPath string) (string, error)
}

func goContext(ctx runconfig.Context) GoContext {
	if g, ok := ctx.Extra.(GoContext); ok {
		return g
	}
	if g, ok := ctx.Extra.(*GoContext); ok && g != nil {
		return *g
	}
	return GoContext{}
}

// Types returns the Go configuration types.
func Types() runconfig.Types {
	return runconfig.Types{TypeApp: handler{}, TypeTest: handler{}, TypeRemote: handler{}}
}

// Tasks returns the Go before-launch tasks.
func Tasks() map[string]runconfig.TaskHandler {
	return map[string]runconfig.TaskHandler{TaskGenerate: generateTask}
}

// generateTask runs `go generate` for the task's package (default ./...).
func generateTask(t runconfig.Task, ctx runconfig.Context) (*runconfig.Cmd, error) {
	pkg := t.Package
	if pkg == "" {
		pkg = "./..."
	}
	args := []string{"generate"}
	if tags := goContext(ctx).Tags; len(tags) > 0 {
		args = append(args, "-tags="+joinComma(tags))
	}
	return &runconfig.Cmd{Path: ctx.Tool, Args: append(args, pkg), Dir: ctx.Root, Env: ctx.Env}, nil
}

func joinComma(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}

type handler struct{}

func (handler) Plan(c runconfig.Config, ctx runconfig.Context) (runconfig.Plan, error) {
	env, err := c.Environ(ctx)
	if err != nil {
		return runconfig.Plan{}, err
	}
	args, err := runconfig.SplitArgs(c.Args)
	if err != nil {
		return runconfig.Plan{}, fmt.Errorf("program arguments: %w", err)
	}
	switch c.Type {
	case TypeApp:
		return planApp(c, ctx, env, args)
	case TypeTest:
		return planTest(c, ctx, env, args)
	case TypeRemote:
		return runconfig.Plan{}, errors.New("Go Remote configurations can only be debugged")
	}
	return runconfig.Plan{}, fmt.Errorf("unknown configuration type %q", c.Type)
}

func (handler) Validate(c runconfig.Config, ctx runconfig.Context) []string { return validate(c, ctx) }

func (handler) Debuggable(runconfig.Config) bool { return true }

func (handler) Runnable(c runconfig.Config) bool { return c.Type != TypeRemote }
