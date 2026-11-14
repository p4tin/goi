// Package agent runs external coding agents that speak the Agent Client
// Protocol: the built-in catalogue, command resolution, npm installs into
// Goi's tools dir, the agent process and its ACP connection, and the
// commands agents run through terminal/* requests.
package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
)

// Definition describes an agent Goi knows how to run.
type Definition struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Package     string   `json:"package"` // npm package; empty for custom agents
	Bin         string   `json:"bin"`     // executable the package installs
	Args        []string `json:"args"`    // arguments that start ACP mode
	Homepage    string   `json:"homepage"`
}

// Catalogue returns the built-in agents.
func Catalogue() []Definition {
	return []Definition{
		{
			ID: "claude", Name: "Claude Code",
			Description: "Anthropic's coding agent, through its ACP adapter. Log in with `claude` first or set ANTHROPIC_API_KEY.",
			Package:     "@agentclientprotocol/claude-agent-acp", Bin: "claude-agent-acp", Args: []string{},
			Homepage: "https://github.com/agentclientprotocol/claude-agent-acp",
		},
		{
			ID: "gemini", Name: "Gemini CLI",
			Description: "Google's Gemini CLI in ACP mode.",
			Package:     "@google/gemini-cli", Bin: "gemini", Args: []string{"--acp"},
			Homepage: "https://github.com/google-gemini/gemini-cli",
		},
		{
			ID: "codex", Name: "Codex",
			Description: "OpenAI's Codex CLI, through its ACP adapter.",
			Package:     "@agentclientprotocol/codex-acp", Bin: "codex-acp", Args: []string{},
			Homepage: "https://github.com/agentclientprotocol/codex-acp",
		},
	}
}

// Lookup returns the catalogue entry for id.
func Lookup(id string) (Definition, bool) {
	for _, d := range Catalogue() {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}

// Definitions returns the catalogue plus custom agents (ids in overrides that
// aren't in the catalogue), catalogue first, custom agents by id.
func Definitions(overrides map[string]string) []Definition {
	out := Catalogue()
	var custom []string
	for id := range overrides {
		if _, ok := Lookup(id); !ok && strings.TrimSpace(overrides[id]) != "" {
			custom = append(custom, id)
		}
	}
	sort.Strings(custom)
	for _, id := range custom {
		out = append(out, Definition{ID: id, Name: id, Description: "Custom agent", Args: []string{}})
	}
	return out
}

// Command is a resolved agent command line.
type Command struct {
	Path   string   `json:"path"`
	Args   []string `json:"args"`
	Source string   `json:"source"` // setting | tools | path
}

// Sources of a resolved command.
const (
	SourceSetting = "setting"
	SourceTools   = "tools"
	SourcePath    = "path"
)

// ErrNotInstalled means the agent's command could not be found.
var ErrNotInstalled = errors.New("agent not installed")

// InstallDir is where Install puts an agent's npm package.
func InstallDir(toolsDir, id string) string {
	return filepath.Join(toolsDir, "agents", id)
}

func binName(bin string) string {
	if runtime.GOOS == "windows" {
		return bin + ".cmd"
	}
	return bin
}

// Resolve finds def's command: the override command line (a setting), the
// npm install in toolsDir, then def.Bin on PATH.
func Resolve(def Definition, override, toolsDir string, lookPath func(string) (string, error)) (Command, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if strings.TrimSpace(override) != "" {
		args, err := SplitCommandLine(override)
		if err != nil {
			return Command{}, fmt.Errorf("agent %s: command line: %w", def.ID, err)
		}
		path := args[0]
		if !strings.ContainsAny(path, `/\`) {
			p, err := lookPath(path)
			if err != nil {
				return Command{}, fmt.Errorf("agent %s: %s: %w", def.ID, path, ErrNotInstalled)
			}
			path = p
		}
		return Command{Path: path, Args: args[1:], Source: SourceSetting}, nil
	}
	if def.Bin == "" {
		return Command{}, fmt.Errorf("agent %s: no command configured: %w", def.ID, ErrNotInstalled)
	}
	args := slices.Clone(def.Args)
	if args == nil {
		args = []string{}
	}
	if toolsDir != "" {
		p := filepath.Join(InstallDir(toolsDir, def.ID), "node_modules", ".bin", binName(def.Bin))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return Command{Path: p, Args: args, Source: SourceTools}, nil
		}
	}
	if p, err := lookPath(def.Bin); err == nil {
		return Command{Path: p, Args: args, Source: SourcePath}, nil
	}
	return Command{}, fmt.Errorf("agent %s: %s: %w", def.ID, def.Bin, ErrNotInstalled)
}
