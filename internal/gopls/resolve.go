package gopls

import (
	"context"

	"github.com/p4tin/goi/internal/toolbin"
)

// ErrNotFound means no gopls binary could be located.
var ErrNotFound = toolbin.ErrNotFound

// Module is the gopls module path installed by Install.
const Module = "golang.org/x/tools/gopls@latest"

// ResolveOptions controls where Resolve looks for gopls.
type ResolveOptions struct {
	// Setting is the gopls.path setting; when set it is the only candidate.
	Setting string
	// LookPath searches $PATH (exec.LookPath when nil).
	LookPath func(file string) (string, error)
	// GOBIN and GOPATH come from `go env`.
	GOBIN, GOPATH string
	// ToolsDir is Goi's managed tools directory.
	ToolsDir string
}

// ExeName is the gopls executable name on this OS.
func ExeName() string { return toolbin.ExeName("gopls") }

// Resolve finds gopls: the gopls.path setting, then $PATH, then $GOBIN,
// then each $GOPATH/bin, then the managed tools dir.
func Resolve(o ResolveOptions) (string, error) {
	return toolbin.Resolve(toolbin.Options{Name: "gopls", Setting: o.Setting, LookPath: o.LookPath, GOBIN: o.GOBIN, GOPATH: o.GOPATH, ToolsDir: o.ToolsDir})
}

// Install runs `go install golang.org/x/tools/gopls@latest` with GOBIN set
// to toolsDir. goBin is the SDK's go binary; env is the project
// environment. It returns the installed binary's path.
func Install(ctx context.Context, goBin string, env []string, toolsDir string) (string, error) {
	return toolbin.Install(ctx, goBin, env, toolsDir, Module, "gopls")
}
