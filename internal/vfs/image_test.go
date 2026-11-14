package vfs

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadDataURL(t *testing.T) {
	dir := t.TempDir()
	png := []byte{0x89, 'P', 'N', 'G', 0, 1, 2}
	os.WriteFile(filepath.Join(dir, "a.PNG"), png, 0o644)
	os.WriteFile(filepath.Join(dir, "b.svg"), []byte("<svg/>"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("x"), 0o644)
	os.Mkdir(filepath.Join(dir, "d.png"), 0o755)

	got, err := ReadDataURL(filepath.Join(dir, "a.PNG"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "data:image/png;base64,"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("got %q", got)
	}
	if b, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, prefix)); string(b) != string(png) {
		t.Errorf("round trip = %v", b)
	}
	if got, _ := ReadDataURL(filepath.Join(dir, "b.svg"), 1<<20); !strings.HasPrefix(got, "data:image/svg+xml;base64,") {
		t.Errorf("svg = %q", got)
	}
	if _, err := ReadDataURL(filepath.Join(dir, "c.txt"), 1<<20); !errors.Is(err, ErrNotImage) {
		t.Errorf("txt err = %v", err)
	}
	if _, err := ReadDataURL(filepath.Join(dir, "d.png"), 1<<20); !errors.Is(err, ErrNotImage) {
		t.Errorf("dir err = %v", err)
	}
	if _, err := ReadDataURL(filepath.Join(dir, "a.PNG"), 3); !errors.Is(err, ErrTooLarge) {
		t.Errorf("limit err = %v", err)
	}
	if _, err := ReadDataURL(filepath.Join(dir, "missing.png"), 1<<20); err == nil {
		t.Error("missing: want error")
	}
}
