package gorun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/p4tin/goi/internal/runconfig"
)

// TestRepoRunConfigsFindTheirPlist plans the repository's own run
// configurations (.goi/runConfigurations: Goi, Pyi) and checks that every
// -sectcreate file exists where the linker looks: relative to the build's
// working directory (the package directory), not the repository root.
func TestRepoRunConfigsFindTheirPlist(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	s, err := runconfig.Open(filepath.Join(repo, ".goi", "runConfigurations"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := runconfig.Context{Root: repo, Tool: "go", TempDir: t.TempDir(), Types: Types(), Extra: GoContext{}}
	checked := 0
	for _, c := range s.List() {
		if c.Type != TypeApp {
			continue
		}
		p, err := c.MakePlan(ctx)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		for _, a := range p.Build.Args {
			_, rest, ok := strings.Cut(a, "-sectcreate,__TEXT,__info_plist,")
			if !ok {
				continue
			}
			path := strings.SplitN(rest, ",", 2)[0]
			if !filepath.IsAbs(path) {
				path = filepath.Join(p.Build.Dir, path)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s: the linker (in %s) can't find its plist: %v", c.Name, p.Build.Dir, err)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no -sectcreate flag found in the repository's run configurations")
	}
}
