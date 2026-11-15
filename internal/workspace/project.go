package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/p4tin/goi/internal/vfs"
)

// Kind is the detected project layout. The app's Detector defines the
// kinds it reports (Goi: "workspace", "module", "multi-module", "gopath");
// KindFolder is the one every app shares.
type Kind string

// KindFolder is a folder with no recognised project files.
const KindFolder Kind = "folder"

// Module is one build unit of the project (a Go module, a Python package…).
type Module struct {
	Path      string `json:"path"`
	Dir       string `json:"dir"`
	GoVersion string `json:"goVersion"`
}

// ParseError locates a problem in a project file. Line is 1-based (0 if
// unknown).
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

// Detector classifies a project root. Detect only reads files; it never
// runs project code. A broken project file is reported in ParseErrors, not
// as an error.
type Detector interface {
	// Detect inspects root; m excludes directories from any nested scan.
	Detect(ctx context.Context, root string, m vfs.Matcher) (Project, error)
	// Watches reports whether a file with this base name affects detection
	// (Goi: go.mod and go.work). A change to one re-runs Detect.
	Watches(name string) bool
}

// FolderDetector treats every root as a plain folder.
type FolderDetector struct{}

// Detect returns the base project for root.
func (FolderDetector) Detect(_ context.Context, root string, _ vfs.Matcher) (Project, error) {
	return NewProject(root)
}

// Watches reports false: nothing changes a plain folder's kind.
func (FolderDetector) Watches(string) bool { return false }

// NewProject returns the folder project for root (made absolute), or an
// error if root isn't a directory. Detectors start from it.
func NewProject(root string) (Project, error) {
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
	return Project{
		Root:         abs,
		Name:         filepath.Base(abs),
		Kind:         KindFolder,
		Modules:      []Module{},
		ContentRoots: []string{abs},
		Excludes:     []string{},
		Hash:         Hash(abs),
		ParseErrors:  []ParseError{},
	}, nil
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
