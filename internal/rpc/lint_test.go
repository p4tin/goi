package rpc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/p4tin/goi/internal/events"
	"github.com/p4tin/goi/internal/settings"
)

func TestLintRunEmitsDiagnostics(t *testing.T) {
	bin := os.Getenv("GOI_GOLANGCI")
	if bin == "" {
		t.Skip("set GOI_GOLANGCI to run golangci-lint integration tests")
	}
	root := runModule(t)
	os.WriteFile(filepath.Join(root, "calc", "bad.go"), []byte("package calc\n\nimport \"os\"\n\nfunc Bad() {\n\tos.Remove(\"x\")\n}\n"), 0o644)
	a, bus := started(t, root)
	a.settings.Set(settings.LayerUser, "lint.golangciPath", bin)
	if st := a.lintSvc.Status(); !st.Found || st.Version == "" {
		t.Fatalf("status = %+v", st)
	}
	res, err := a.lintSvc.Run("package", filepath.Join(root, "calc", "bad.go"))
	if err != nil || res.Error != "" || res.Issues == 0 {
		t.Fatalf("run = %+v %v", res, err)
	}
	evs := bus.Events(events.TopicLintDiagnostics)
	if len(evs) != 1 {
		t.Fatalf("events = %d", len(evs))
	}
	ev := evs[0].Payload.(LintDiagnosticsEvent)
	ds := ev.Files[filepath.Join(root, "calc", "bad.go")]
	if len(ev.Dirs) != 1 || ev.Dirs[0] != filepath.Join(root, "calc") || len(ds) == 0 || ds[0].Source != "golangci-lint (errcheck)" || ds[0].Range.Start.Line != 5 {
		t.Fatalf("event = %+v", ev)
	}
	// Disabled in settings: no run.
	a.settings.Set(settings.LayerUser, "lint.enabled", false)
	if _, err := a.lintSvc.Run("project", root); err == nil {
		t.Fatal("disabled lint must not run")
	}
}

func TestLintMissingBinary(t *testing.T) {
	a, _ := started(t, runModule(t))
	a.settings.Set(settings.LayerUser, "lint.golangciPath", filepath.Join(t.TempDir(), "nope"))
	if st := a.lintSvc.Status(); st.Found {
		t.Fatalf("status = %+v", st)
	}
	if _, err := a.lintSvc.Run("project", ""); err == nil {
		t.Fatal("run without golangci-lint must fail")
	}
}

func TestModuleDirFor(t *testing.T) {
	root := runModule(t)
	if got := moduleDirFor(filepath.Join(root, "calc", "calc.go"), root); got != root {
		t.Fatalf("moduleDirFor = %q", got)
	}
	if got := moduleDirFor(t.TempDir(), root); got != "" {
		t.Fatalf("outside = %q", got)
	}
}

func TestModServiceModulesFromGoSum(t *testing.T) {
	root := runModule(t)
	os.WriteFile(filepath.Join(root, "go.sum"), []byte("github.com/stretchr/testify v1.9.0 h1:x=\n"), 0o644)
	a, _ := started(t, root)
	got := a.modSvc.Modules("github.com/stre")
	found := false
	for _, m := range got {
		if m == "github.com/stretchr/testify" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Modules = %v", got)
	}
}
