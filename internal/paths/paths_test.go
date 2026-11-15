package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/p4tin/goi/internal/product"
)

func TestMain(m *testing.M) {
	product.Set(product.Identity{Name: "Goi", Program: "goi", DirName: "Goi", DataVersion: "0.1", HomeEnv: "GOI_HOME"})
	os.Exit(m.Run())
}

func TestDirsFollowIdentity(t *testing.T) {
	old := product.Current()
	t.Cleanup(func() { product.Set(old) })
	product.Set(product.Identity{Name: "Zed", DirName: "Zed", DataVersion: "2.3", HomeEnv: "ZED_HOME"})
	t.Setenv("GOI_HOME", "/nope")
	t.Setenv("ZED_HOME", "")
	d, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(d) != "2.3" || filepath.Base(filepath.Dir(d)) != "Zed" {
		t.Fatalf("ConfigDir = %q, want …/Zed/2.3", d)
	}
	zedHome := t.TempDir() // absolute on every OS ("/tmp/zed" isn't on Windows)
	t.Setenv("ZED_HOME", zedHome)
	if d, _ := CacheDir(); d != filepath.Join(zedHome, "cache") {
		t.Fatalf("CacheDir with ZED_HOME = %q", d)
	}
	product.Set(product.Identity{})
	if _, err := ConfigDir(); err == nil {
		t.Fatal("ConfigDir without an identity should fail")
	}
}

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
	b, err := os.ReadFile("../../apps/goi/build/config.yml")
	if err != nil {
		t.Skip(err)
	}
	if ConfigVersion(b) == "" {
		t.Fatal("build/config.yml has no info.version")
	}
}
