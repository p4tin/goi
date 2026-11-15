package pyrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/p4tin/goi/internal/pack"
	"github.com/p4tin/goi/internal/runconfig"
)

func TestMarkers(t *testing.T) {
	src := "import sys\n\ndef main():\n    pass\n\nif __name__ == \"__main__\":\n    main()\n"
	m := Run{}.Markers("/p/app.py", src)
	if len(m) != 1 || m[0].Line != 6 || m[0].Kind != "main" {
		t.Fatalf("markers = %+v", m)
	}
	if m := (Run{}).Markers("/p/app.py", "if '__main__' == __name__:\n"); len(m) != 1 {
		t.Fatalf("reversed guard = %+v", m)
	}
	if m := (Run{}).Markers("/p/notes.txt", src); len(m) != 0 {
		t.Fatalf("non-Python = %+v", m)
	}
}

func TestContextConfigAndPlan(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "app.py"), []byte("print('hi')\n"), 0o644)
	rel := func(p string) string { r, _ := filepath.Rel(root, p); return filepath.ToSlash(r) }
	c, err := Run{}.ContextConfig(pack.ContextRequest{Path: filepath.Join(root, "app.py"), Kind: "main"}, rel)
	if err != nil || c.Type != TypePython || c.Script != "app.py" || c.Name != "app.py" {
		t.Fatalf("context config = %+v, %v", c, err)
	}
	if _, err := (Run{}).ContextConfig(pack.ContextRequest{Path: root, Kind: "dir"}, rel); err == nil {
		t.Fatal("dir: want error")
	}
	ctx := runconfig.Context{Root: root, Tool: "/usr/bin/python3", Env: []string{"PATH=/usr/bin"}, Types: Run{}.Types()}
	c.Args = `-v "a b"`
	p, err := c.MakePlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Exec.Path != "/usr/bin/python3" || p.Exec.Dir != root || strings.Join(p.Exec.Args, "|") != filepath.Join(root, "app.py")+"|-v|a b" || p.Build != nil {
		t.Fatalf("plan = %+v", p.Exec)
	}
	if errs := c.Validate(ctx, nil); len(errs) != 0 {
		t.Fatalf("valid config: %v", errs)
	}
	if errs := (runconfig.Config{Name: "x", Type: TypePython, Script: "missing.py"}).Validate(ctx, nil); len(errs) == 0 || !strings.Contains(errs[0], "does not exist") {
		t.Fatalf("missing script: %v", errs)
	}
	if ctx.Types.Debuggable(c) || !ctx.Types.Runnable(c) {
		t.Fatal("Python runs but isn't debuggable yet")
	}
}
