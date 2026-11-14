package rpc

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/p4tin/goi/internal/settings"
	"github.com/p4tin/goi/internal/vfs"
)

// FileService exposes project file operations. Mutations are confined to the
// project's content roots (NFR-4).
type FileService struct {
	app *App
}

// OpenedFile is a file loaded for editing.
type OpenedFile struct {
	Meta    vfs.FileMeta `json:"meta"`
	Content string       `json:"content"`
}

// List returns the children of dir, honouring project excludes.
func (s *FileService) List(ctx context.Context, dir string) ([]vfs.Entry, error) {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return nil, err
	}
	root, ok := sess.contains(dir)
	if !ok {
		return nil, fmt.Errorf("files: %s is outside the project", dir)
	}
	entries, err := vfs.List(root, dir, sess.manager.Matcher())
	if entries == nil {
		entries = []vfs.Entry{}
	}
	return entries, err
}

// Read loads a text file using the editor size limits from the calling
// window's settings.
func (s *FileService) Read(ctx context.Context, path string) (OpenedFile, error) {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return OpenedFile{}, err
	}
	e := w.settings.Effective().Editor
	lim := vfs.Limits{HighlightBytes: int64(e.MaxHighlightSizeKB) * 1024, OpenBytes: int64(e.MaxOpenSizeKB) * 1024}
	meta, content, err := vfs.ReadText(path, lim)
	return OpenedFile{Meta: meta, Content: content}, err
}

// maxImageBytes caps images loaded by the Markdown preview.
const maxImageBytes = 10 << 20

// ReadImage returns an image inside the calling window's project as a data
// URL, for the Markdown preview. Symlinks are resolved first, so a link
// inside the project can't expose a file outside it.
func (s *FileService) ReadImage(ctx context.Context, path string) (string, error) {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	inside := false
	for _, r := range sess.manager.Project().ContentRoots {
		if rr, err := filepath.EvalSymlinks(r); err == nil && within(rr, resolved) {
			inside = true
			break
		}
	}
	if !inside {
		return "", fmt.Errorf("files: %s is outside the project", abs)
	}
	return vfs.ReadDataURL(resolved, maxImageBytes)
}

// Write saves content (LF-normalized) restoring the file's EOL and BOM.
func (s *FileService) Write(ctx context.Context, path string, content string, meta vfs.FileMeta) (vfs.FileMeta, error) {
	if err := s.guard(ctx, path); err != nil && !s.isSettingsFile(ctx, path) {
		return vfs.FileMeta{}, err
	}
	return vfs.WriteText(path, content, meta)
}

// Create makes a new file (with content) or directory.
func (s *FileService) Create(ctx context.Context, path string, isDir bool, content string) error {
	if err := s.guard(ctx, path); err != nil {
		return err
	}
	return vfs.Create(path, isDir, content)
}

// CreateGoFile creates <dir>/<name>.go with a package clause matching the
// other Go files in dir, or derived from the directory name. It returns the
// new file's path.
func (s *FileService) CreateGoFile(ctx context.Context, dir, name string) (string, error) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".go")
	if name == "" || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("files: invalid Go file name %q", name)
	}
	path := filepath.Join(dir, name+".go")
	if err := s.guard(ctx, path); err != nil {
		return "", err
	}
	pkg := packageFor(dir, strings.HasSuffix(name, "_test"))
	return path, vfs.Create(path, false, "package "+pkg+"\n")
}

// Rename moves a file or directory within the project.
func (s *FileService) Rename(ctx context.Context, from, to string) error {
	if err := s.guard(ctx, from); err != nil {
		return err
	}
	if err := s.guard(ctx, to); err != nil {
		return err
	}
	return vfs.Rename(from, to)
}

// Trash moves path to the OS trash.
func (s *FileService) Trash(ctx context.Context, path string) error {
	if err := s.guard(ctx, path); err != nil {
		return err
	}
	return vfs.Trash(path)
}

// Reveal shows path in the OS file manager.
func (s *FileService) Reveal(path string) error {
	return vfs.Reveal(path)
}

// CopyPath puts path on the clipboard.
func (s *FileService) CopyPath(path string) error {
	if s.app.wapp == nil || !s.app.wapp.Clipboard.SetText(path) {
		return errors.New("could not set the clipboard")
	}
	return nil
}

// isSettingsFile reports whether path is the calling window's user or project
// settings file, which may be edited directly even when outside the project.
func (s *FileService) isSettingsFile(ctx context.Context, path string) bool {
	w, err := s.app.windowFor(ctx)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, p := range []string{w.settings.Path(settings.LayerUser), w.settings.Path(settings.LayerProject)} {
		if p != "" && filepath.Clean(p) == abs {
			return true
		}
	}
	return false
}

// guard rejects paths outside the calling window's project content roots,
// and content roots themselves.
func (s *FileService) guard(ctx context.Context, path string) error {
	sess, err := s.app.sessionFor(ctx)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, ok := sess.contains(abs)
	if !ok {
		return fmt.Errorf("files: %s is outside the project", abs)
	}
	if filepath.Clean(abs) == filepath.Clean(root) {
		return fmt.Errorf("files: refusing to modify the content root %s", root)
	}
	return nil
}

// packageFor returns the package name used by existing Go files in dir
// (ignoring the _test external package unless test), else one derived from
// the directory name.
func packageFor(dir string, test bool) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || (strings.HasSuffix(n, "_test.go") && !test) {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, n), nil, parser.PackageClauseOnly)
		if err == nil && f.Name != nil {
			return strings.TrimSuffix(f.Name.Name, "_test")
		}
	}
	return sanitizePackage(filepath.Base(dir))
}

func sanitizePackage(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-' || r == '.' || r == ' ' || r == '_':
			// dropped
		}
	}
	out := b.String()
	if out == "" {
		return "main"
	}
	if unicode.IsDigit(rune(out[0])) {
		out = "p" + out
	}
	return out
}
