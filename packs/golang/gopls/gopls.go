// Package gopls is Goi's language server: locating and installing gopls,
// its settings and the Go language ids. The core's langserver package
// supervises it.
package gopls

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/toolbin"
	"github.com/p4tin/goi/packs/golang/gosdk"
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
	// ToolsDir is the app's managed tools directory.
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

// LanguageID returns gopls's language id for a path ("" if gopls does not
// handle it).
func LanguageID(path string) string {
	switch base := filepath.Base(path); {
	case strings.HasSuffix(base, ".go"):
		return "go"
	case base == "go.mod":
		return "go.mod"
	case base == "go.work":
		return "go.work"
	case base == "go.sum":
		return "go.sum"
	case strings.HasSuffix(base, ".tmpl") || strings.HasSuffix(base, ".gotmpl"):
		return "gotmpl"
	}
	return ""
}

// Server is gopls as a pack.LanguageServer.
type Server struct{}

var _ pack.LanguageServer = Server{}

// Name is "gopls".
func (Server) Name() string { return "gopls" }

// Enabled reports the gopls.enabled setting.
func (Server) Enabled(s settings.Settings) bool { return s.Gopls.Enabled }

// RestartKey covers the go.* and gopls.* settings.
func (Server) RestartKey(s settings.Settings) string {
	b, _ := json.Marshal(struct {
		Go    settings.Go
		Gopls settings.Gopls
	}{s.Go, s.Gopls})
	return string(b)
}

// Command resolves gopls with the project's go env (GOBIN, GOPATH).
func (Server) Command(ctx context.Context, p pack.ProjectEnv, _ []string, toolsDir string) (pack.ServerCommand, error) {
	opts := ResolveOptions{Setting: p.Settings.Gopls.Path, ToolsDir: toolsDir}
	if r, err := gosdk.ForProject(ctx, p.Settings, p.Root); err == nil {
		if ge, err := r.GoEnv(ctx); err == nil {
			opts.GOBIN, opts.GOPATH = ge["GOBIN"], ge["GOPATH"]
		}
	}
	bin, err := Resolve(opts)
	if err != nil {
		return pack.ServerCommand{}, err
	}
	return pack.ServerCommand{Bin: bin}, nil
}

// Settings is the gopls configuration: Goi's defaults, the gopls.settings
// map, and buildFlags from go.tags and go.vendor.
func (Server) Settings(p pack.ProjectEnv) map[string]any {
	s := p.Settings
	cfg := map[string]any{
		"usePlaceholders":    s.Gopls.UsePlaceholders,
		"staticcheck":        s.Gopls.Staticcheck,
		"completeUnimported": true,
		"deepCompletion":     true,
		"matcher":            "Fuzzy",
		"linksInHover":       true,
		"hoverKind":          "FullDocumentation",
	}
	for k, v := range s.Gopls.Settings {
		cfg[k] = v
	}
	var flags []string
	if len(s.Go.Tags) > 0 {
		flags = append(flags, "-tags="+strings.Join(s.Go.Tags, ","))
	}
	if m := gosdk.ModFlag(s.Go.Vendor); m != "" {
		flags = append(flags, m)
	}
	if len(flags) > 0 {
		cfg["buildFlags"] = flags
	}
	return cfg
}

// Section returns the whole configuration for every section (gopls asks
// for "gopls").
func (Server) Section(_ string, cfg map[string]any) any { return cfg }

// LanguageID is the package LanguageID.
func (Server) LanguageID(path string) string { return LanguageID(path) }

// Install runs `go install` for gopls with the project's SDK.
func (Server) Install(ctx context.Context, p pack.ProjectEnv, env []string, toolsDir string) (string, error) {
	r, err := gosdk.ForProject(ctx, p.Settings, p.Root)
	if err != nil {
		return "", err
	}
	return Install(ctx, r.SDK.GoBin, r.Environ(), toolsDir)
}

// InstallHint describes Install.
func (Server) InstallHint() string { return "go install " + Module }
