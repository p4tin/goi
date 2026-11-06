package rpc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const newProjectTimeout = 15 * time.Second

// NewProject creates parentDir/name with a go.mod for modulePath and a
// main.go, and returns the new directory. It uses `go mod init` when a Go SDK
// is available and writes a minimal go.mod otherwise.
func (s *AppService) NewProject(parentDir, name, modulePath string) (string, error) {
	return s.app.newProject(parentDir, name, modulePath)
}

func (a *App) newProject(parentDir, name, modulePath string) (string, error) {
	name = strings.TrimSpace(name)
	modulePath = strings.TrimSpace(modulePath)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("new project: invalid name %q", name)
	}
	if modulePath == "" || strings.ContainsAny(modulePath, " \t") {
		return "", fmt.Errorf("new project: invalid module path %q", modulePath)
	}
	parent, err := filepath.Abs(parentDir)
	if err != nil {
		return "", fmt.Errorf("new project: %w", err)
	}
	if st, err := os.Stat(parent); err != nil || !st.IsDir() {
		return "", fmt.Errorf("new project: %s is not a folder", parent)
	}
	dir := filepath.Join(parent, name)
	if _, err := os.Lstat(dir); err == nil {
		return "", fmt.Errorf("new project: %s already exists", dir)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", fmt.Errorf("new project: %w", err)
	}

	if err := a.goModInit(dir, modulePath); err != nil {
		a.log.Info("go mod init unavailable, writing go.mod", "err", err)
		content := fmt.Sprintf("module %s\n\ngo %s\n", modulePath, goMajorMinor(runtime.Version()))
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644); err != nil {
			return "", fmt.Errorf("new project: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {\n}\n"), 0o644); err != nil {
		return "", fmt.Errorf("new project: %w", err)
	}
	return dir, nil
}

func (a *App) goModInit(dir, modulePath string) error {
	ctx, cancel := context.WithTimeout(a.ctx, newProjectTimeout)
	defer cancel()
	r, err := a.runner(ctx)
	if err != nil {
		return err
	}
	r.Dir = dir
	out, err := r.Command(ctx, "mod", "init", modulePath).CombinedOutput()
	if err != nil {
		os.Remove(filepath.Join(dir, "go.mod"))
		return fmt.Errorf("go mod init: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// goMajorMinor turns "go1.26.0" (or "go1.26rc1", "devel ...") into "1.26".
func goMajorMinor(v string) string {
	v = strings.TrimPrefix(v, "go")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return "1.26"
	}
	minor := parts[1]
	for i, c := range minor {
		if c < '0' || c > '9' {
			minor = minor[:i]
			break
		}
	}
	if minor == "" {
		return "1.26"
	}
	return parts[0] + "." + minor
}
