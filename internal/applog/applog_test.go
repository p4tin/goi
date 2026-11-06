package applog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingWriterRollsOverAndKeepsLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.log")
	w, err := NewRotatingWriter(p, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	line := bytes.Repeat([]byte("a"), 60)
	for i := 0; i < 5; i++ {
		if _, err := w.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"x.log", "x.log.1", "x.log.2"} {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
		if st.Size() != 60 {
			t.Errorf("%s size = %d, want 60", name, st.Size())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "x.log.3")); !os.IsNotExist(err) {
		t.Errorf("x.log.3 should not exist, err=%v", err)
	}
}

func TestRotatingWriterAppendsToExisting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.log")
	if err := os.WriteFile(p, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := NewRotatingWriter(p, 1<<20, 1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(w, "new\n")
	w.Close()
	b, _ := os.ReadFile(p)
	if string(b) != "old\nnew\n" {
		t.Errorf("content = %q", b)
	}
}

func TestNewWritesJSON(t *testing.T) {
	dir := t.TempDir()
	log, c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.Info("hello", "k", 1)
	c.Close()
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(b), &rec); err != nil {
		t.Fatalf("not JSON: %q", b)
	}
	if rec["msg"] != "hello" {
		t.Errorf("msg = %v", rec["msg"])
	}
}
