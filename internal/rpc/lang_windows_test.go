package rpc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/p4tin/goi/internal/settings"
)

// TestSearchRoutesByWindow checks that file search, Find in Path and Replace
// in Path act on the calling window's project only.
func TestSearchRoutesByWindow(t *testing.T) {
	a, w1, w2, _, _ := twoWindows(t)
	s1, _ := w1.current()
	s2, _ := w2.current()
	ctxA := withWindow(context.Background(), w1)
	ctxB := withWindow(context.Background(), w2)

	for _, c := range []struct {
		ctx        context.Context
		root, name string
	}{{ctxA, s1.root, "util.go"}, {ctxB, s2.root, "util.go"}} {
		var res FileSearchResult
		eventuallyRPC(t, func() bool {
			res, _ = a.searchSvc.Files(c.ctx, "util", 10, nil)
			return res.Index.Ready && len(res.Matches) > 0
		})
		for _, m := range res.Matches {
			if !within(c.root, m.Path) {
				t.Errorf("Files for %s returned %s", c.root, m.Path)
			}
		}

		found, err := a.searchSvc.FindInPath(c.ctx, FindQuery{Pattern: "helpers"})
		if err != nil {
			t.Fatal(err)
		}
		if len(found.Matches) != 1 || found.Matches[0].Path != filepath.Join(c.root, "pkg", "util", c.name) {
			t.Errorf("FindInPath for %s = %+v", c.root, found.Matches)
		}
	}

	// Window A cannot replace in window B's files.
	utilB := filepath.Join(s2.root, "pkg", "util", "util.go")
	if _, err := a.searchSvc.ReplaceInPath(ctxA, FindQuery{Pattern: "helpers", Replace: "x"}, []string{utilB}); err == nil {
		t.Error("ReplaceInPath from window A wrote window B's file")
	}
	if b, _ := os.ReadFile(utilB); string(b) != "package helpers\n" {
		t.Errorf("window B's file = %q", b)
	}
	if n, err := a.searchSvc.ReplaceInPath(ctxB, FindQuery{Pattern: "helpers", Replace: "x"}, []string{utilB}); err != nil || n != 1 {
		t.Errorf("ReplaceInPath from window B: n = %d, err = %v", n, err)
	}

	// With two windows a call must say which window it comes from.
	if _, err := a.searchSvc.Files(context.Background(), "util", 10, nil); !errors.Is(err, errNoWindow) {
		t.Errorf("Files without a window: err = %v, want errNoWindow", err)
	}
}

// TestFormatUsesCallingWindowSettings checks that Format reads save.format
// from the calling window's project settings.
func TestFormatUsesCallingWindowSettings(t *testing.T) {
	a, w1, w2, _, _ := twoWindows(t)
	if err := w2.settings.Set(settings.LayerProject, "save.format", "none"); err != nil {
		t.Fatal(err)
	}
	src := "package m\nfunc  f( ) {  }\n"
	s1, _ := w1.current()
	s2, _ := w2.current()

	resA, err := a.editorSvc.Format(withWindow(context.Background(), w1), filepath.Join(s1.root, "x.go"), src)
	if err != nil || !resA.Changed {
		t.Errorf("window A (goimports): %+v, err = %v", resA, err)
	}
	resB, err := a.editorSvc.Format(withWindow(context.Background(), w2), filepath.Join(s2.root, "x.go"), src)
	if err != nil || resB.Changed {
		t.Errorf("window B (none): %+v, err = %v", resB, err)
	}
}

// TestLangStatusRoutesByWindow checks that LangService resolves the calling
// window's session (gopls is not started in tests).
func TestLangStatusRoutesByWindow(t *testing.T) {
	a, w1, w2, _, _ := twoWindows(t)
	if err := w1.closeProject(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.langSvc.lang(withWindow(context.Background(), w1)); !errors.Is(err, errNoProject) {
		t.Errorf("window A without a project: err = %v, want errNoProject", err)
	}
	if _, err := a.langSvc.lang(withWindow(context.Background(), w2)); !errors.Is(err, errLangNotRunning) {
		t.Errorf("window B with a project: err = %v, want errLangNotRunning", err)
	}
}
