// Package pack defines an application pack: everything that makes one
// product (Goi for Go, Pyi for Python) out of the shared IDE core. A pack
// carries the product identity and the language tooling (project detection,
// toolchain, language servers, formatter, linter, run types, debugger).
//
// Each app's main package builds a Pack and hands it to boot.Main. Core code
// reads capabilities only through the interfaces here; it never branches on
// which pack is running, and nothing in internal/ imports a pack. A nil
// capability means the app doesn't have it: core hides its UI.
package pack

import (
	"context"
	"log/slog"

	"github.com/p4tin/goi/internal/lint"
	"github.com/p4tin/goi/internal/product"
	"github.com/p4tin/goi/internal/runconfig"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/workspace"
)

// Identity is the product identity (name, ids, directories).
type Identity = product.Identity

// Pack is one application: identity plus capabilities.
type Pack struct {
	Identity Identity
	// Project classifies project roots (Goi: go.work/go.mod). Nil treats
	// every project as a plain folder.
	Project workspace.Detector
	// Toolchain provides the environment of programs started for a project
	// (Goi: the Go SDK; Pyi: the interpreter). Nil gives them the app's own
	// environment.
	Toolchain Toolchain
	// Server is the project's language server (Goi: gopls). Nil means no
	// code intelligence.
	Server LanguageServer
	// Formatter formats buffers of the pack's language on Reformat and on
	// save (Goi: gofmt/goimports). Nil leaves only the core formatters
	// (JSON, YAML).
	Formatter Formatter
	// Linter runs the pack's linter (Goi: golangci-lint). Nil hides
	// linting.
	Linter Linter
	// Run adds configuration types, gutter markers and context runs. Nil
	// leaves the built-in Shell and Makefile configurations.
	Run Run
	// Debug is the debug adapter (Goi: Delve). Nil hides debugging.
	Debug DebugAdapter
	// SettingsDefaults are TOML defaults for the pack's own settings
	// sections (Pyi: [python]). Their values reach the pack through
	// settings.Settings.Value and the frontend through the effective
	// settings JSON.
	SettingsDefaults []byte
	// Services returns the app's own bound services, each already wrapped
	// with application.NewService by the app's main package (packs don't
	// import Wails). Nil means none.
	Services func(Host) []any
}

// ProjectEnv is what a toolchain needs to know about a project.
type ProjectEnv struct {
	// Root is the project root ("" when no project is open).
	Root string
	// Settings are the effective settings of the project's window.
	Settings settings.Settings
}

// Toolchain is a language toolchain.
type Toolchain interface {
	// Env returns the environment of programs started for the project:
	// language servers, run configurations, terminals, linters and agents.
	Env(ctx context.Context, p ProjectEnv) ([]string, error)
}

// RunMarker is a runnable spot in a file, shown in the editor gutter.
type RunMarker struct {
	// Kind is the marker kind the frontend pack understands (Goi: main,
	// test, bench, fuzz, example).
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Line is 1-based.
	Line int `json:"line"`
}

// ContextRequest asks for a temporary configuration for a gutter marker,
// a file or a directory.
type ContextRequest struct {
	// Path is the file (or directory for kind "dir").
	Path string `json:"path"`
	// Kind is a RunMarker kind, "file" or "dir".
	Kind string `json:"kind"`
	// Name is the marker name (function, test…).
	Name string `json:"name"`
}

// Run is a pack's run configuration support.
type Run interface {
	// Types are the configuration types the pack adds.
	Types() runconfig.Types
	// Tasks are the before-launch task types the pack adds.
	Tasks() map[string]runconfig.TaskHandler
	// Context fills the planning context for the project (Tool, Env, Extra).
	Context(ctx context.Context, p ProjectEnv, tempDir string) (runconfig.Context, error)
	// Markers returns the runnable spots of a file; nil or empty for none.
	Markers(path, content string) []RunMarker
	// ContextConfig builds the temporary configuration for req. rel renders
	// a path relative to the project root ("./x/y", or "." for the root).
	ContextConfig(req ContextRequest, rel func(path string) string) (runconfig.Config, error)
	// IsTest reports whether c's output is a test event stream.
	IsTest(c runconfig.Config) bool
}

// Session is a pack service's view of the project open in a window.
type Session interface {
	Root() string
	Project() workspace.Project
	Settings() settings.Settings
	// State returns the session's value for key, creating it with create
	// on first use. Pack services keep per-project caches here.
	State(key string, create func() any) any
	// Env is the toolchain environment of the project.
	Env(ctx context.Context) []string
}

// Host is the core API pack services use. The bound method's context
// identifies the calling window.
type Host interface {
	// Session returns the project open in the calling window.
	Session(ctx context.Context) (Session, error)
	// Settings returns the calling window's effective settings.
	Settings(ctx context.Context) (settings.Settings, error)
	// SetSetting stores key in the project settings when a project is open,
	// otherwise in the user settings. A nil value clears it.
	SetSetting(ctx context.Context, key string, value any) error
	// Env is the toolchain environment of the calling window (its project's
	// when one is open).
	Env(ctx context.Context) []string
	// Background is the app's lifetime context: work that outlives a call
	// must not use the call's context.
	Background() context.Context
	Log() *slog.Logger
	// ToolsDir is where the app installs tools.
	ToolsDir() string
}

// ServerCommand starts a language server.
type ServerCommand struct {
	Bin  string
	Args []string
}

// LanguageServer is a language server the core supervises (one per open
// project).
type LanguageServer interface {
	// Name is the server's display name ("gopls").
	Name() string
	// Enabled reports whether settings turn the server on.
	Enabled(s settings.Settings) bool
	// RestartKey summarizes the settings that require a restart when they
	// change.
	RestartKey(s settings.Settings) string
	// Command locates the server; an error wrapping toolbin.ErrNotFound
	// means it isn't installed (the UI offers Install).
	Command(ctx context.Context, p ProjectEnv, env []string, toolsDir string) (ServerCommand, error)
	// Settings is the server configuration (initializationOptions and
	// workspace/configuration).
	Settings(p ProjectEnv) map[string]any
	// Section answers workspace/configuration for one section.
	Section(section string, settings map[string]any) any
	// LanguageID maps a path to the server's LSP language id ("" = not
	// handled).
	LanguageID(path string) string
	// Install installs the server into toolsDir and returns its path.
	Install(ctx context.Context, p ProjectEnv, env []string, toolsDir string) (string, error)
	// InstallHint describes what Install runs, for progress messages.
	InstallHint() string
}

// FormatResult is a formatted buffer. A syntax error is reported in
// SyntaxError (1-based line and column), not as an error.
type FormatResult struct {
	Content     string
	Changed     bool
	SyntaxError *SyntaxError
}

// SyntaxError is the first parse error of a buffer.
type SyntaxError struct {
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Msg  string `json:"msg"`
}

// Formatter formats source files of the pack's language.
type Formatter interface {
	// Handles reports whether the formatter formats path.
	Handles(path string) bool
	// Format formats content with mode (the save.format setting: Goi's
	// goimports | gofmt | none) in project p.
	Format(ctx context.Context, p FormatContext, path, content, mode string) (FormatResult, error)
}

// FormatContext is the project a buffer is formatted in.
type FormatContext struct {
	// Project is the open project (zero when none).
	Project workspace.Project
}

// LintJob is one linter invocation: Targets run in Dir, and the results
// replace earlier diagnostics of files under Covered (recursively when
// Recursive).
type LintJob struct {
	Dir       string
	Targets   []string
	Covered   string
	Recursive bool
}

// Linter is the pack's linter.
type Linter interface {
	// Name is the linter's display name ("golangci-lint").
	Name() string
	// Enabled reports whether settings turn linting on.
	Enabled(s settings.Settings) bool
	// Resolve locates the linter; an error wrapping toolbin.ErrNotFound
	// means it isn't installed.
	Resolve(ctx context.Context, p ProjectEnv, toolsDir string) (string, error)
	// Version reports the binary's version.
	Version(ctx context.Context, bin string) (string, error)
	// Jobs plans a run: scope "project" lints every unit of the project,
	// "package" the unit containing path.
	Jobs(scope, path string, p workspace.Project) ([]LintJob, error)
	// Run runs one job and returns its issues.
	Run(ctx context.Context, bin string, job LintJob, env []string, s settings.Settings) ([]lint.Issue, error)
	// Install installs the linter into toolsDir and returns its path.
	Install(ctx context.Context, p ProjectEnv, toolsDir string) (string, error)
	// InstallHint describes what Install runs.
	InstallHint() string
}

// DebugAdapter starts debug adapters (DAP servers) for configurations.
type DebugAdapter interface {
	// Name is the debugger's display name ("Delve").
	Name() string
	// ID is the DAP adapterID ("go").
	ID() string
	// Resolve locates the adapter program; an error wrapping
	// toolbin.ErrNotFound means it isn't installed.
	Resolve(ctx context.Context, p ProjectEnv, toolsDir string) (ServerCommand, error)
	// Attach reports whether c attaches to a running adapter at addr
	// (host:port) instead of launching one.
	Attach(c runconfig.Config) (addr string, ok bool)
	// LaunchArgs returns the launch request arguments for c. bin is a temp
	// path for the built program.
	LaunchArgs(c runconfig.Config, ctx runconfig.Context, bin string) (map[string]any, error)
	// Install installs the adapter into toolsDir and returns its path.
	Install(ctx context.Context, p ProjectEnv, toolsDir string) (string, error)
	// InstallHint describes what Install runs.
	InstallHint() string
}
