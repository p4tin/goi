package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/packs/python/pyenv"
)

// ProjectService creates Python projects.
type ProjectService struct {
	h pack.Host
}

// NewProjectService returns the project service.
func NewProjectService(h pack.Host) *ProjectService { return &ProjectService{h: h} }

// distName is a valid PyPI distribution name.
var distName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// NewProject creates parentDir/name with a pyproject.toml and a main.py,
// plus a .venv made with the calling window's interpreter when venv is set,
// and returns the new directory.
func (s *ProjectService) NewProject(ctx context.Context, parentDir, name string, venv bool) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("new project: invalid name %q", name)
	}
	if !distName.MatchString(name) {
		return "", fmt.Errorf("new project: %q is not a valid Python project name (letters, digits, '.', '_' and '-')", name)
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
	pyproject := fmt.Sprintf("[project]\nname = %q\nversion = \"0.1.0\"\nrequires-python = \">=3.9\"\ndependencies = []\n", name)
	main := "def main() -> None:\n    print(\"Hello from " + name + "!\")\n\n\nif __name__ == \"__main__\":\n    main()\n"
	for f, content := range map[string]string{"pyproject.toml": pyproject, "main.py": main, ".gitignore": ".venv/\n__pycache__/\n*.egg-info/\n"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(content), 0o644); err != nil {
			return "", fmt.Errorf("new project: %w", err)
		}
	}
	if venv {
		if err := s.makeVenv(ctx, dir); err != nil {
			s.h.Log().Warn("new project venv", "err", err)
			return dir, fmt.Errorf("created %s, but not its virtual environment: %w", dir, err)
		}
	}
	return dir, nil
}

func (s *ProjectService) makeVenv(callCtx context.Context, dir string) error {
	st, err := s.h.Settings(callCtx)
	if err != nil {
		return err
	}
	py, err := pyenv.Interpreter(st, "")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.h.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, py, "-m", "venv", filepath.Join(dir, ".venv")).CombinedOutput()
	if err != nil {
		return fmt.Errorf("python -m venv: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
