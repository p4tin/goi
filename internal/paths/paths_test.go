package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelativeGOIHomeBecomesAbsolute(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GOI_HOME", ".goi-home")
	for name, fn := range map[string]func() (string, error){"config": ConfigDir, "cache": CacheDir} {
		d, err := fn()
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(d) || filepath.Base(d) != name || filepath.Base(filepath.Dir(d)) != ".goi-home" {
			t.Errorf("%s dir = %q, want an absolute path ending in .goi-home/%s", name, d, name)
		}
	}
}

func TestConfigVersion(t *testing.T) {
	cfg := []byte(`version: '3'

# comment
info:
  companyName: "Goi"
  version: "0.1.3" # The application version
ios:
  version: "9.9.9"
`)
	if got := ConfigVersion(cfg); got != "0.1.3" {
		t.Fatalf("ConfigVersion = %q, want 0.1.3", got)
	}
	if got := ConfigVersion([]byte("version: '3'\ninfo:\n  name: x\n")); got != "" {
		t.Fatalf("ConfigVersion without info.version = %q, want empty", got)
	}
}

func TestConfigVersionRepo(t *testing.T) {
	b, err := os.ReadFile("../../build/config.yml")
	if err != nil {
		t.Skip(err)
	}
	if ConfigVersion(b) == "" {
		t.Fatal("build/config.yml has no info.version")
	}
}
