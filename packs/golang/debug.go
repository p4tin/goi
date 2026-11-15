package golang

import (
	"context"
	"net"
	"strconv"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/internal/toolbin"
	"github.com/p4tin/goi/packs/golang/gorun"
	"github.com/p4tin/goi/packs/golang/gosdk"
)

// DlvModule is what "Install Delve" installs.
const DlvModule = "github.com/go-delve/delve/cmd/dlv@latest"

// delve is Delve (`dlv dap`) as a pack.DebugAdapter.
type delve struct{}

func (delve) Name() string        { return "Delve" }
func (delve) ID() string          { return "go" }
func (delve) InstallHint() string { return "go install " + DlvModule }

// Resolve finds dlv: debug.dlvPath, PATH, GOBIN, GOPATH/bin, then the tools
// dir, and runs it as `dlv dap`.
func (delve) Resolve(ctx context.Context, p pack.ProjectEnv, toolsDir string) (pack.ServerCommand, error) {
	opts := toolbin.Options{Name: "dlv", Setting: p.Settings.Debug.DlvPath, ToolsDir: toolsDir}
	opts.GOBIN, opts.GOPATH = goEnvPaths(ctx, p)
	bin, err := toolbin.Resolve(opts)
	if err != nil {
		return pack.ServerCommand{}, err
	}
	return pack.ServerCommand{Bin: bin, Args: []string{"dap", "--listen=127.0.0.1:0"}}, nil
}

// Attach reports Go Remote configurations (a headless Delve at host:port).
func (delve) Attach(c runconfig.Config) (string, bool) {
	if c.Type != gorun.TypeRemote {
		return "", false
	}
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)), true
}

// LaunchArgs returns dlv's launch arguments for Go Build and Go Test.
func (delve) LaunchArgs(c runconfig.Config, ctx runconfig.Context, bin string) (map[string]any, error) {
	return gorun.DebugLaunch(c, ctx, bin)
}

// Install runs `go install` for Delve with the project's SDK.
func (delve) Install(ctx context.Context, p pack.ProjectEnv, toolsDir string) (string, error) {
	r, err := gosdk.ForProject(ctx, p.Settings, p.Root)
	if err != nil {
		return "", err
	}
	return toolbin.Install(ctx, r.SDK.GoBin, r.Environ(), toolsDir, DlvModule, "dlv")
}
