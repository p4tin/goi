// Package pyright is Pyi's language server: basedpyright
// (basedpyright-langserver --stdio). Install puts it in its own virtual
// environment under the tools folder (the PyPI package bundles Node.js, so
// nothing else is needed) and links the server into the tools bin folder.
package pyright

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/toolbin"
	"github.com/p4tin/goi/packs/python/pyenv"
)

// Binary is the language server executable's base name.
const Binary = "basedpyright-langserver"

// Package is the PyPI package Install installs.
const Package = "basedpyright"

// Server is basedpyright as a pack.LanguageServer.
type Server struct{}

var _ pack.LanguageServer = Server{}

// Name is "basedpyright".
func (Server) Name() string { return "basedpyright" }

// Enabled is the python.lsp.enabled setting (default on).
func (Server) Enabled(s settings.Settings) bool {
	v, ok := s.Value("python.lsp.enabled").(bool)
	return !ok || v
}

// RestartKey covers the [python] settings (the interpreter and server).
func (Server) RestartKey(s settings.Settings) string {
	b, _ := json.Marshal(s.Value("python"))
	return string(b)
}

// Command resolves the server: python.lsp.path, PATH, the tools folder,
// then the virtual environment Install creates.
func (Server) Command(_ context.Context, p pack.ProjectEnv, _ []string, toolsDir string) (pack.ServerCommand, error) {
	setting := p.Settings.String("python.lsp.path")
	bin, err := toolbin.Resolve(toolbin.Options{Name: Binary, Setting: setting, ToolsDir: toolsDir})
	if err != nil && setting == "" {
		if venvBin := filepath.Join(scriptsDir(VenvDir(toolsDir)), toolbin.ExeName(Binary)); toolbin.IsExec(venvBin) {
			bin, err = venvBin, nil
		}
	}
	if err != nil {
		return pack.ServerCommand{}, err
	}
	return pack.ServerCommand{Bin: bin, Args: []string{"--stdio"}}, nil
}

// scriptsDir is a virtual environment's executables folder.
func scriptsDir(venv string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venv, "Scripts")
	}
	return filepath.Join(venv, "bin")
}

// Settings tells basedpyright which interpreter to analyse with, plus the
// python.lsp.analysis map (basedpyright.analysis settings).
func (Server) Settings(p pack.ProjectEnv) map[string]any {
	analysis := map[string]any{"autoSearchPaths": true, "useLibraryCodeForTypes": true, "diagnosticMode": "openFilesOnly"}
	if m, ok := p.Settings.Value("python.lsp.analysis").(map[string]any); ok {
		for k, v := range m {
			analysis[k] = v
		}
	}
	py := map[string]any{}
	if interp, err := pyenv.Interpreter(p.Settings, p.Root); err == nil {
		py["pythonPath"] = interp
		if venv := pyenv.VenvOf(interp); venv != "" {
			py["venvPath"], py["venv"] = filepath.Dir(venv), filepath.Base(venv)
		}
	}
	return map[string]any{
		"python":       py,
		"basedpyright": map[string]any{"analysis": analysis},
	}
}

// Section answers workspace/configuration ("python", "basedpyright.analysis"
// …) with that part of the configuration.
func (Server) Section(section string, cfg map[string]any) any {
	if section == "" {
		return cfg
	}
	var cur any = cfg
	for _, part := range strings.Split(section, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return map[string]any{}
		}
		cur = m[part]
	}
	if cur == nil {
		return map[string]any{}
	}
	return cur
}

// LanguageID is "python" for .py and .pyi files.
func (Server) LanguageID(path string) string { return LanguageID(path) }

// LanguageID returns "python" for Python sources and stubs.
func LanguageID(path string) string {
	switch filepath.Ext(path) {
	case ".py", ".pyi", ".pyw":
		return "python"
	}
	return ""
}

// InstallHint describes Install.
func (Server) InstallHint() string { return "pip install " + Package }

// VenvDir is where Install creates basedpyright's virtual environment.
func VenvDir(toolsDir string) string {
	return filepath.Join(filepath.Dir(toolsDir), "venvs", Package)
}

// Install creates a virtual environment with the project's interpreter,
// installs basedpyright into it and links the server into toolsDir.
func (Server) Install(ctx context.Context, p pack.ProjectEnv, env []string, toolsDir string) (string, error) {
	py, err := pyenv.Interpreter(p.Settings, p.Root)
	if err != nil {
		return "", err
	}
	venv := VenvDir(toolsDir)
	if err := os.MkdirAll(filepath.Dir(venv), 0o755); err != nil {
		return "", err
	}
	run := func(name string, args ...string) error {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s %s: %w: %s", filepath.Base(name), strings.Join(args, " "), err, lastLines(string(out), 6))
		}
		return nil
	}
	if err := run(py, "-m", "venv", "--clear", venv); err != nil {
		return "", err
	}
	scripts := scriptsDir(venv)
	vpy := filepath.Join(scripts, toolbin.ExeName("python"))
	if err := run(vpy, "-m", "pip", "install", "--disable-pip-version-check", "--upgrade", Package); err != nil {
		return "", err
	}
	src := filepath.Join(scripts, toolbin.ExeName(Binary))
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("install %s: %s missing after pip install", Package, src)
	}
	if runtime.GOOS == "windows" {
		// pip's .exe launchers find their venv from their own path, so a
		// copy in the tools folder would not run; Command finds it here.
		return src, nil
	}
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(toolsDir, Binary)
	os.Remove(dst)
	return dst, os.Symlink(src, dst)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
