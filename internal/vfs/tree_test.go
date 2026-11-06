package vfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListOrderAndExcludes(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"file10.go", "file2.go", "B.go", "a.go", "x.log"} {
		touch(t, filepath.Join(root, f))
	}
	for _, d := range []string{"zdir", "Adir", ".git"} {
		os.Mkdir(filepath.Join(root, d), 0o755)
	}
	m, _ := NewMatcher([]string{".git", "*.log"})
	got, err := List(root, root, m)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range got {
		names = append(names, e.Name)
	}
	want := []string{"Adir", "zdir", "a.go", "B.go", "file2.go", "file10.go"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
	if !got[0].IsDir || got[2].IsDir || got[2].Size != 1 {
		t.Errorf("entry kinds/sizes wrong: %+v", got)
	}
}

func TestListExcludesRelativeToRoot(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "build", "out", "x"))
	m, _ := NewMatcher([]string{"build/out"})
	got, err := List(root, filepath.Join(root, "build"), m)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected build/out excluded, got %+v", got)
	}
}

func TestListSymlinks(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "real"), 0o755)
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "broken"))
	got, err := List(root, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, e := range got {
		kinds[e.Name] = e.IsDir
	}
	if !kinds["link"] || kinds["broken"] {
		t.Errorf("kinds = %v", kinds)
	}
}

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"file2", "file10", true},
		{"file10", "file2", false},
		{"a", "B", true},
		{"file02", "file2", true}, // numerically equal; byte order breaks the tie
		{"x", "x1", true},
	}
	for _, tt := range tests {
		if got := naturalLess(tt.a, tt.b); got != tt.want {
			t.Errorf("naturalLess(%q,%q) = %v", tt.a, tt.b, got)
		}
	}
}

func TestCreate(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "a", "b", "main.go")
	if err := Create(f, false, "package b\n"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	if string(b) != "package b\n" {
		t.Errorf("content = %q", b)
	}
	if err := Create(f, false, ""); !errors.Is(err, fs.ErrExist) {
		t.Errorf("want ErrExist, got %v", err)
	}
	d := filepath.Join(root, "x", "y")
	if err := Create(d, true, ""); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(d); err != nil || !st.IsDir() {
		t.Errorf("dir not created: %v", err)
	}
}

func TestRename(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")
	touch(t, a)
	touch(t, b)
	if err := Rename(a, b); !errors.Is(err, fs.ErrExist) {
		t.Errorf("want ErrExist, got %v", err)
	}
	c := filepath.Join(root, "c.txt")
	if err := Rename(a, c); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c); err != nil {
		t.Error("c.txt missing")
	}
}

func TestTrashFreedesktop(t *testing.T) {
	src := t.TempDir()
	data := t.TempDir()
	now := time.Date(2026, 11, 6, 9, 30, 0, 0, time.Local)

	p := filepath.Join(src, "my file.txt")
	touch(t, p)
	if err := trashFreedesktop(p, data, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("source still exists")
	}
	if _, err := os.Stat(filepath.Join(data, "Trash", "files", "my file.txt")); err != nil {
		t.Error("trashed file missing:", err)
	}
	info, err := os.ReadFile(filepath.Join(data, "Trash", "info", "my file.txt.trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(info)
	if !strings.HasPrefix(s, "[Trash Info]\n") || !strings.Contains(s, "my%20file.txt") ||
		!strings.Contains(s, "DeletionDate=2026-11-06T09:30:00\n") {
		t.Errorf("info = %q", s)
	}

	// A second item with the same name gets a numeric suffix.
	touch(t, p)
	if err := trashFreedesktop(p, data, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "Trash", "files", "my file.txt.2")); err != nil {
		t.Error("collision file missing:", err)
	}
	if _, err := os.Stat(filepath.Join(data, "Trash", "info", "my file.txt.2.trashinfo")); err != nil {
		t.Error("collision info missing:", err)
	}
}

func TestTrashFreedesktopMissing(t *testing.T) {
	if err := trashFreedesktop(filepath.Join(t.TempDir(), "nope"), t.TempDir(), time.Now()); err == nil {
		t.Error("expected error for missing source")
	}
}
