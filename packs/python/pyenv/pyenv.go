// Package pyenv is Pyi's view of a Python project: which folders are
// Python projects, which interpreter a project uses (the python.interpreter
// setting, the project's virtual environment, then python3 on PATH) and the
// environment programs started for the project get (the venv activated).
package pyenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/vfs"
	"github.com/p4tin/goi/internal/workspace"
)

// Kinds of Python project.
const (
	// KindProject has a pyproject.toml, setup.py/setup.cfg or requirements
	// file at the root.
	KindProject workspace.Kind = "python"
	// KindVenv is a folder with only a virtual environment.
	KindVenv workspace.Kind = "venv"
)

// Markers are the root files that make a folder a Python project; a change
// to one re-runs detection.
var Markers = []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt", "requirements-dev.txt", "Pipfile"}

// VenvDirs are the virtual environment folders looked for at the root.
var VenvDirs = []string{".venv", "venv", "env"}

// Detector is the workspace.Detector for Python projects.
type Detector struct{}

var _ workspace.Detector = Detector{}

// projectName finds `name = "..."` in pyproject.toml's [project] table.
var projectName = regexp.MustCompile(`(?ms)^\[project\]\s*$.*?^name\s*=\s*["']([^"']+)["']`)

// Detect classifies root: KindProject with one module (the pyproject name,
// else the folder name) when a marker exists, KindVenv with only a venv,
// otherwise a plain folder.
func (Detector) Detect(_ context.Context, root string, _ vfs.Matcher) (workspace.Project, error) {
	p, err := workspace.NewProject(root)
	if err != nil {
		return p, err
	}
	for _, m := range Markers {
		if isFile(filepath.Join(p.Root, m)) {
			p.Kind = KindProject
			break
		}
	}
	if p.Kind == KindProject {
		name := p.Name
		if b, err := os.ReadFile(filepath.Join(p.Root, "pyproject.toml")); err == nil {
			if m := projectName.FindSubmatch(b); m != nil {
				name = string(m[1])
			}
		}
		p.Modules = append(p.Modules, workspace.Module{Path: name, Dir: p.Root})
		return p, nil
	}
	if VenvPython(p.Root) != "" {
		p.Kind = KindVenv
	}
	return p, nil
}

// Watches reports the project markers.
func (Detector) Watches(name string) bool {
	for _, m := range Markers {
		if name == m {
			return true
		}
	}
	return false
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// venvPython is a virtual environment's interpreter path.
func venvPython(venv string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venv, "Scripts", "python.exe")
	}
	return filepath.Join(venv, "bin", "python")
}

// VenvPython returns the interpreter of the first virtual environment at
// root (.venv, venv, env), or "".
func VenvPython(root string) string {
	if root == "" {
		return ""
	}
	for _, d := range VenvDirs {
		if py := venvPython(filepath.Join(root, d)); isFile(py) && isFile(filepath.Join(root, d, "pyvenv.cfg")) {
			return py
		}
	}
	return ""
}

// ErrNoInterpreter means no Python interpreter was found.
var ErrNoInterpreter = errors.New("no Python interpreter found; install Python 3 or set python.interpreter")

// LookPath finds a program on PATH (exec.LookPath; tests replace it).
var LookPath = exec.LookPath

// Interpreter returns the project's Python: the python.interpreter setting
// (relative paths against root), else the project's venv, else python3
// (python on Windows) on PATH.
func Interpreter(s settings.Settings, root string) (string, error) {
	if set := s.String("python.interpreter"); set != "" {
		if !filepath.IsAbs(set) && root != "" {
			set = filepath.Join(root, set)
		}
		if !isFile(set) {
			return "", fmt.Errorf("python.interpreter %q does not exist", set)
		}
		return set, nil
	}
	if py := VenvPython(root); py != "" {
		return py, nil
	}
	names := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		names = []string{"python", "python3", "py"}
	}
	for _, n := range names {
		if p, err := LookPath(n); err == nil {
			return p, nil
		}
	}
	return "", ErrNoInterpreter
}

// VenvOf returns the virtual environment an interpreter belongs to (the
// folder holding pyvenv.cfg), or "".
func VenvOf(python string) string {
	dir := filepath.Dir(filepath.Dir(python)) // <venv>/bin/python
	if isFile(filepath.Join(dir, "pyvenv.cfg")) {
		return dir
	}
	return ""
}

// Environ returns base with python's venv activated (VIRTUAL_ENV set, its
// scripts folder first on PATH, PYTHONHOME removed). A python outside a
// venv leaves base as is apart from PYTHONUNBUFFERED, which keeps program
// output streaming into the console.
func Environ(base []string, python string) []string {
	env := setEnv(append([]string{}, base...), "PYTHONUNBUFFERED", "1")
	venv := VenvOf(python)
	if venv == "" {
		return env
	}
	env = unsetEnv(env, "PYTHONHOME")
	env = setEnv(env, "VIRTUAL_ENV", venv)
	path := filepath.Dir(python)
	if old := getEnv(env, "PATH"); old != "" {
		path += string(os.PathListSeparator) + old
	}
	return setEnv(env, "PATH", path)
}

func envKeyEq(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func getEnv(env []string, key string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && envKeyEq(k, key) {
			return v
		}
	}
	return ""
}

func setEnv(env []string, key, val string) []string {
	for i, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && envKeyEq(k, key) {
			env[i] = key + "=" + val
			return env
		}
	}
	return append(env, key+"="+val)
}

func unsetEnv(env []string, key string) []string {
	out := env[:0]
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && envKeyEq(k, key) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Toolchain is the project interpreter as a pack.Toolchain.
type Toolchain struct{}

var _ pack.Toolchain = Toolchain{}

// Env is the app environment with the project's venv activated.
func (Toolchain) Env(_ context.Context, p pack.ProjectEnv) ([]string, error) {
	py, err := Interpreter(p.Settings, p.Root)
	if err != nil {
		return nil, err
	}
	return Environ(os.Environ(), py), nil
}
