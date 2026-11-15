// Package godetect classifies Go project roots: go.work workspaces, go.mod
// modules, folders of nested modules and GOPATH packages.
package godetect

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/p4tin/goi/internal/vfs"
	"github.com/p4tin/goi/internal/workspace"
)

// Kinds of Go project.
const (
	KindWorkspace   workspace.Kind = "workspace"    // go.work at the root
	KindModule      workspace.Kind = "module"       // go.mod at the root
	KindMultiModule workspace.Kind = "multi-module" // go.mod files below the root only
	KindGopath      workspace.Kind = "gopath"       // inside $GOPATH/src
	KindFolder                     = workspace.KindFolder
)

type (
	// Kind is the project kind.
	Kind = workspace.Kind
	// Module is one Go module.
	Module = workspace.Module
	// ParseError is a go.mod/go.work problem.
	ParseError = workspace.ParseError
	// Project is the detected project.
	Project = workspace.Project
)

// nestedDepth bounds the search for go.mod files below the root.
const nestedDepth = 4

// Detector is the workspace.Detector for Go projects.
type Detector struct {
	// GOPATH lists GOPATH entries (a root inside one's src is KindGopath).
	GOPATH []string
}

// Detect classifies root (see the package Detect).
func (d Detector) Detect(ctx context.Context, root string, m vfs.Matcher) (workspace.Project, error) {
	return Detect(ctx, root, d.GOPATH, m)
}

// Watches reports whether name is go.mod or go.work.
func (Detector) Watches(name string) bool { return name == "go.mod" || name == "go.work" }

// Hash is workspace.Hash.
func Hash(root string) string { return workspace.Hash(root) }

// Detect inspects root and classifies it. It only reads files; it never runs
// project code. gopath lists GOPATH entries; m excludes directories from the
// nested go.mod scan. A broken root go.work or go.mod yields KindFolder with
// the problem in ParseErrors rather than an error.
func Detect(ctx context.Context, root string, gopath []string, m vfs.Matcher) (Project, error) {
	if m == nil {
		m = vfs.MatchNone
	}
	p, err := workspace.NewProject(root)
	if err != nil {
		return Project{}, err
	}
	abs := p.Root

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
