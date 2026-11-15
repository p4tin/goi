package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackSections(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.toml")
	os.WriteFile(user, []byte("[python]\ninterpreter = \"/u/python\"\n"), 0o644)
	st, err := Load(user, "", nil, []byte("[python]\ninterpreter = \"\"\n[python.lsp]\npath = \"\"\nenabled = true\n"))
	if err != nil {
		t.Fatal(err)
	}
	eff := st.Effective()
	if eff.String("python.interpreter") != "/u/python" || eff.Value("python.lsp.enabled") != true || eff.Value("nope.x") != nil {
		t.Fatalf("pack values = %v %v", eff.Value("python.interpreter"), eff.Value("python.lsp.enabled"))
	}
	b, err := json.Marshal(eff)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"extra":{"python":{`) || !strings.Contains(string(b), `"editor":{`) {
		t.Fatalf("json = %s", b)
	}
	if err := st.Set(LayerUser, "python.lsp.path", "/x/basedpyright-langserver"); err != nil {
		t.Fatal(err)
	}
	if st.Effective().String("python.lsp.path") != "/x/basedpyright-langserver" {
		t.Fatal("Set on a pack key not effective")
	}
}
