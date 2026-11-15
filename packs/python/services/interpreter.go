// Package services holds Pyi's own bound services. apps/pyi binds them
// (packs don't import Wails).
package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/packs/python/pyenv"
)

// InterpreterService reports and selects the project's Python interpreter.
type InterpreterService struct {
	h pack.Host
}

// NewInterpreterService returns the interpreter service.
func NewInterpreterService(h pack.Host) *InterpreterService { return &InterpreterService{h: h} }

// Interpreter describes one Python interpreter.
type Interpreter struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	// Venv is the virtual environment folder ("" for a system Python).
	Venv string `json:"venv"`
}

// InterpreterStatus is the project's interpreter, or why there is none.
type InterpreterStatus struct {
	OK          bool        `json:"ok"`
	Interpreter Interpreter `json:"interpreter"`
	// FromSetting reports that python.interpreter chose it.
	FromSetting bool   `json:"fromSetting"`
	Problem     string `json:"problem"`
}

// version runs `python --version` ("Python 3.12.1" → "3.12.1").
func version(ctx context.Context, python string) string {
	vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(vctx, python, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "Python"))
}

func describe(ctx context.Context, python string) Interpreter {
	return Interpreter{Path: python, Version: version(ctx, python), Venv: pyenv.VenvOf(python)}
}

// Status resolves the calling window's interpreter.
func (s *InterpreterService) Status(ctx context.Context) InterpreterStatus {
	st, err := s.h.Settings(ctx)
	if err != nil {
		return InterpreterStatus{Problem: err.Error()}
	}
	root := ""
	if sess, err := s.h.Session(ctx); err == nil {
		root = sess.Root()
	}
	py, err := pyenv.Interpreter(st, root)
	if err != nil {
		return InterpreterStatus{Problem: err.Error()}
	}
	return InterpreterStatus{OK: true, Interpreter: describe(s.h.Background(), py), FromSetting: st.String("python.interpreter") != ""}
}

// Discover lists the project's virtual environments and the Pythons on
// PATH and in the usual install folders.
func (s *InterpreterService) Discover(ctx context.Context) []Interpreter {
	seen := map[string]bool{}
	var paths []string
	add := func(p string) {
		if p == "" {
			return
		}
		key := p
		if real, err := filepath.EvalSymlinks(p); err == nil {
			key = real
		}
		if seen[key] {
			return
		}
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			return
		}
		seen[key] = true
		paths = append(paths, p)
	}
	if sess, err := s.h.Session(ctx); err == nil {
		add(pyenv.VenvPython(sess.Root()))
	}
	for _, n := range []string{"python3", "python"} {
		if p, err := pyenv.LookPath(n); err == nil {
			add(p)
		}
	}
	var globs []string
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			globs = append(globs, filepath.Join(la, "Programs", "Python", "Python3*", "python.exe"))
		}
	} else {
		globs = append(globs, "/usr/bin/python3", "/usr/local/bin/python3*", "/opt/homebrew/bin/python3*", "/Library/Frameworks/Python.framework/Versions/*/bin/python3")
		if home, err := os.UserHomeDir(); err == nil {
			globs = append(globs, filepath.Join(home, ".pyenv", "versions", "*", "bin", "python3"))
		}
	}
	for _, g := range globs {
		ms, _ := filepath.Glob(g)
		sort.Strings(ms)
		for _, m := range ms {
			if !strings.HasSuffix(m, "-config") {
				add(m)
			}
		}
	}
	out := make([]Interpreter, 0, len(paths))
	for _, p := range paths {
		out = append(out, describe(s.h.Background(), p))
	}
	return out
}

// Select stores path as python.interpreter (project settings when a
// project is open). An empty path goes back to automatic selection.
func (s *InterpreterService) Select(ctx context.Context, path string) error {
	var v any = path
	if path == "" {
		v = nil
	}
	return s.h.SetSetting(ctx, "python.interpreter", v)
}
