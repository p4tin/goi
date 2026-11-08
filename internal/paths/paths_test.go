package paths

import (
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
