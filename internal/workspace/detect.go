package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/p4tin/goi/internal/vfs"
)

// Kind is the detected project layout.
type Kind string

const (
	KindWorkspace   Kind = "workspace"    // go.work at the root
	KindModule      Kind = "module"       // go.mod at the root
	KindMultiModule Kind = "multi-module" // go.mod files below the root only
	KindGopath      Kind = "gopath"       // inside $GOPATH/src
	KindFolder      Kind = "folder"       // anything else
)

// nestedDepth bounds the search for go.mod files below the root.
const nestedDepth = 4

// Module is one Go module in the project.
type Module struct {
	Path      string `json:"path"`
	Dir       string `json:"dir"`
	GoVersion string `json:"goVersion"`
}

// ParseError locates a go.mod/go.work problem. Line is 1-based (0 if unknown).
type ParseError struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Msg  string `json:"msg"`
}

// Project is the open project model.
type Project struct {
	Root         string       `json:"root"`
	Name         string       `json:"name"`
	Kind         Kind         `json:"kind"`
	Modules      []Module     `json:"modules"`
	ContentRoots []string     `json:"contentRoots"`
	Excludes     []string     `json:"excludes"`
	Hash         string       `json:"hash"`
	ParseErrors  []ParseError `json:"parseErrors"`
}

// Detect inspects root and classifies it. It only reads files; it never runs
// project code. gopath lists GOPATH entries; m excludes directories from the
// nested go.mod scan. A broken root go.work or go.mod yields KindFolder with
// the problem in ParseErrors rather than an error.
func Detect(ctx context.Context, root string, gopath []string, m vfs.Matcher) (Project, error) {
	if m == nil {
		m = vfs.MatchNone
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Project{}, fmt.Errorf("workspace: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return Project{}, fmt.Errorf("workspace: %w", err)
	}
	if !st.IsDir() {
		return Project{}, fmt.Errorf("workspace: %s is not a directory", abs)
	}
	p := Project{
		Root:         abs,
		Name:         filepath.Base(abs),
		Kind:         KindFolder,
		Modules:      []Module{},
		ContentRoots: []string{abs},
		Excludes:     []string{},
		Hash:         Hash(abs),
		ParseErrors:  []ParseError{},
	}

	workPath := filepath.Join(abs, "go.work")
	if exists(workPath) {
		mods, perr := parseWork(workPath)
		if perr != nil {
			p.ParseErrors = append(p.ParseErrors, *perr)
			return p, nil
		}
		p.Kind = KindWorkspace
		for _, dir := range mods {
			mod, perr := readModule(filepath.Join(dir, "go.mod"))
			if perr != nil {
				p.ParseErrors = append(p.ParseErrors, *perr)
				continue
			}
			p.Modules = append(p.Modules, mod)
		}
		return p, nil
	}

	modPath := filepath.Join(abs, "go.mod")
	if exists(modPath) {
		mod, perr := readModule(modPath)
		if perr != nil {
			p.ParseErrors = append(p.ParseErrors, *perr)
			return p, nil
		}
		p.Kind = KindModule
		p.Modules = append(p.Modules, mod)
		return p, nil
	}

	nested, perrs, err := findNested(ctx, abs, m)
	if err != nil {
		return Project{}, err
	}
	p.ParseErrors = append(p.ParseErrors, perrs...)
	if len(nested) > 0 {
		p.Kind = KindMultiModule
		p.Modules = nested
		return p, nil
	}

	if inGopath(abs, gopath) {
		p.Kind = KindGopath
	}
	return p, nil
}

// Hash returns a stable 16-hex-digit key for a project root, used to key
// per-project user state. Symlinks are resolved so aliases share state.
func Hash(root string) string {
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(sum[:8])
}

func exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func parseWork(path string) ([]string, *ParseError) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{File: path, Msg: err.Error()}
	}
	wf, err := modfile.ParseWork(path, data, nil)
	if err != nil {
		return nil, toParseError(path, err)
	}
	dir := filepath.Dir(path)
	var dirs []string
	for _, u := range wf.Use {
		d := filepath.FromSlash(u.Path)
		if !filepath.IsAbs(d) {
			d = filepath.Join(dir, d)
		}
		dirs = append(dirs, filepath.Clean(d))
	}
	return dirs, nil
}

func readModule(path string) (Module, *ParseError) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Module{}, &ParseError{File: path, Msg: err.Error()}
	}
	f, err := modfile.ParseLax(path, data, nil)
	if err != nil {
		return Module{}, toParseError(path, err)
	}
	mod := Module{Dir: filepath.Dir(path)}
	if f.Module != nil {
		mod.Path = f.Module.Mod.Path
	}
	if f.Go != nil {
		mod.GoVersion = f.Go.Version
	}
	if mod.Path == "" {
		return Module{}, &ParseError{File: path, Line: 1, Msg: "missing module directive"}
	}
	return mod, nil
}

func toParseError(path string, err error) *ParseError {
	var list modfile.ErrorList
	if errors.As(err, &list) && len(list) > 0 {
		e := list[0]
		return &ParseError{File: path, Line: e.Pos.Line, Msg: e.Err.Error()}
	}
	return &ParseError{File: path, Msg: err.Error()}
}

func findNested(ctx context.Context, root string, m vfs.Matcher) ([]Module, []ParseError, error) {
	var mods []Module
	var perrs []ParseError
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !d.IsDir() || p == root {
			if !d.IsDir() && d.Name() == "go.mod" && filepath.Dir(p) != root {
				mod, perr := readModule(p)
				if perr != nil {
					perrs = append(perrs, *perr)
				} else {
					mods = append(mods, mod)
				}
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		name := d.Name()
		if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
			m.Match(rel, true) || strings.Count(rel, "/") >= nestedDepth {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("workspace: scan %s: %w", root, err)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Dir < mods[j].Dir })
	return mods, perrs, nil
}

func inGopath(root string, gopath []string) bool {
	for _, gp := range gopath {
		if gp == "" {
			continue
		}
		src := filepath.Join(gp, "src")
		rel, err := filepath.Rel(src, root)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}
