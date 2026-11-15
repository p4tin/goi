package pyenv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/p4tin/goi/internal/settings"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
}

// fakeVenv makes root/<dir> look like a virtual environment.
func fakeVenv(t *testing.T, root, dir string) string {
	t.Helper()
	write(t, filepath.Join(root, dir, "pyvenv.cfg"), "home = /usr/bin\n")
	py := venvPython(filepath.Join(root, dir))
	write(t, py, "")
	return py
}

func withSettings(t *testing.T, toml string) settings.Settings {
	t.Helper()
	dir := t.TempDir()
	user := filepath.Join(dir, "user.toml")
	write(t, user, toml)
	st, err := settings.Load(user, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return st.Effective()
}

func TestDetect(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	p, err := Detector{}.Detect(ctx, root, nil)
	if err != nil || p.Kind != "folder" {
		t.Fatalf("empty = %+v, %v", p, err)
	}
	fakeVenv(t, root, ".venv")
	if p, _ := (Detector{}).Detect(ctx, root, nil); p.Kind != KindVenv {
		t.Fatalf("venv only = %s", p.Kind)
	}
	write(t, filepath.Join(root, "pyproject.toml"), "[build-system]\nrequires = []\n\n[project]\nname = \"demo-app\"\nversion = \"1\"\n")
	p, _ = Detector{}.Detect(ctx, root, nil)
	if p.Kind != KindProject || len(p.Modules) != 1 || p.Modules[0].Path != "demo-app" || p.Modules[0].Dir != p.Root {
		t.Fatalf("pyproject = %+v", p)
	}
	other := t.TempDir()
	write(t, filepath.Join(other, "requirements.txt"), "requests\n")
	if p, _ := (Detector{}).Detect(ctx, other, nil); p.Kind != KindProject || p.Modules[0].Path != filepath.Base(p.Root) {
		t.Fatalf("requirements = %+v", p)
	}
	if !(Detector{}).Watches("pyproject.toml") || (Detector{}).Watches("main.py") {
		t.Fatal("Watches")
	}
}

func TestInterpreterOrder(t *testing.T) {
	root := t.TempDir()
	old := LookPath
	t.Cleanup(func() { LookPath = old })
	LookPath = func(string) (string, error) { return "", errors.New("none") }
	if _, err := Interpreter(settings.Settings{}, root); !errors.Is(err, ErrNoInterpreter) {
		t.Fatalf("nothing = %v", err)
	}
	LookPath = func(n string) (string, error) { return "/usr/bin/" + n, nil }
	if py, _ := Interpreter(settings.Settings{}, root); !strings.HasPrefix(filepath.Base(py), "python") {
		t.Fatalf("PATH = %q", py)
	}
	venvPy := fakeVenv(t, root, ".venv")
	if py, _ := Interpreter(settings.Settings{}, root); py != venvPy {
		t.Fatalf("venv = %q, want %q", py, venvPy)
	}
	custom := filepath.Join(root, "tools", "py")
	write(t, custom, "")
	s := withSettings(t, "[python]\ninterpreter = \"tools/py\"\n")
	if py, _ := Interpreter(s, root); py != custom {
		t.Fatalf("setting = %q", py)
	}
	s = withSettings(t, "[python]\ninterpreter = \"/nope/python\"\n")
	if _, err := Interpreter(s, root); err == nil {
		t.Fatal("missing setting path: want error")
	}
}

func TestEnviron(t *testing.T) {
	root := t.TempDir()
	py := fakeVenv(t, root, ".venv")
	env := Environ([]string{"PATH=/usr/bin", "PYTHONHOME=/x"}, py)
	get := func(k string) string {
		for _, kv := range env {
			if a, b, _ := strings.Cut(kv, "="); a == k {
				return b
			}
		}
		return ""
	}
	if get("VIRTUAL_ENV") != filepath.Join(root, ".venv") || !strings.HasPrefix(get("PATH"), filepath.Dir(py)+string(os.PathListSeparator)) || get("PYTHONHOME") != "" || get("PYTHONUNBUFFERED") != "1" {
		t.Fatalf("env = %v", env)
	}
	plain := Environ([]string{"PATH=/usr/bin"}, "/usr/bin/python3")
	if strings.Contains(strings.Join(plain, ";"), "VIRTUAL_ENV") {
		t.Fatalf("system python env = %v", plain)
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(py, `Scripts\python.exe`) {
		t.Fatalf("windows venv python = %s", py)
	}
}
