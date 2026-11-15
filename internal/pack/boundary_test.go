package pack

import (
	"os/exec"
	"strings"
	"testing"
)

// TestPackBoundaries keeps the core independent of the packs (R1.2, R8.2):
// no non-test package under internal/ imports packs/ or apps/, packs don't
// import each other or apps/, and each app imports only its own pack.
func TestPackBoundaries(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}}:{{join .Imports \" \"}}", "github.com/p4tin/goi/...").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	const mod = "github.com/p4tin/goi/"
	area := func(pkg string) (kind, name string) {
		rest, ok := strings.CutPrefix(pkg, mod)
		if !ok {
			return "", ""
		}
		parts := strings.Split(rest, "/")
		switch parts[0] {
		case "internal":
			return "core", ""
		case "packs", "apps":
			if len(parts) > 1 {
				return parts[0], parts[1]
			}
		}
		return parts[0], ""
	}
	appPack := map[string]string{"goi": "golang", "pyi": "python"}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, imps, _ := strings.Cut(line, ":")
		kind, name := area(pkg)
		for _, imp := range strings.Fields(imps) {
			ik, iname := area(imp)
			switch {
			case kind == "core" && (ik == "packs" || ik == "apps"):
				t.Errorf("core package %s imports %s", pkg, imp)
			case kind == "packs" && ik == "packs" && iname != name:
				t.Errorf("pack package %s imports another pack: %s", pkg, imp)
			case kind == "packs" && ik == "apps":
				t.Errorf("pack package %s imports an app: %s", pkg, imp)
			case kind == "apps" && ik == "packs" && appPack[name] != "" && iname != appPack[name]:
				t.Errorf("app %s imports %s (its pack is %s)", pkg, imp, appPack[name])
			}
		}
	}
}
