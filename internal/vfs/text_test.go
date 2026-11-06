package vfs

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadTextAndRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		eol      string
		bom      bool
		finalNL  bool
		wantText string
	}{
		{"lf", "a\nb\n", "\n", false, true, "a\nb\n"},
		{"crlf", "a\r\nb\r\n", "\r\n", false, true, "a\nb\n"},
		{"cr", "a\rb\r", "\r", false, true, "a\nb\n"},
		{"mixed majority crlf", "a\r\nb\r\nc\n", "\r\n", false, true, "a\nb\nc\n"},
		{"mixed tie defaults lf", "a\r\nb\n", "\n", false, true, "a\nb\n"},
		{"bom", "\xEF\xBB\xBFa\nb\n", "\n", true, true, "a\nb\n"},
		{"bom crlf", "\xEF\xBB\xBFa\r\n", "\r\n", true, true, "a\n"},
		{"no final newline", "a\nb", "\n", false, false, "a\nb"},
		{"no newline at all", "abc", "\n", false, false, "abc"},
		{"empty", "", "\n", false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeFixture(t, []byte(tt.raw))
			meta, text, err := ReadText(p, DefaultLimits)
			if err != nil {
				t.Fatal(err)
			}
			if meta.Binary || meta.EOL != tt.eol || meta.BOM != tt.bom || meta.FinalNewline != tt.finalNL || meta.Mode != ModeNormal {
				t.Errorf("meta = %+v", meta)
			}
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if meta.Size != int64(len(tt.raw)) {
				t.Errorf("size = %d", meta.Size)
			}

			// Round trip unchanged content must reproduce the file exactly,
			// except mixed endings, which are unified to the majority.
			if _, err := WriteText(p, text, meta); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(p)
			want := tt.raw
			if strings.HasPrefix(tt.name, "mixed") {
				want = strings.ReplaceAll(tt.wantText, "\n", tt.eol)
			}
			if string(got) != want {
				t.Errorf("round trip = %q, want %q", got, want)
			}
		})
	}
}

func TestWriteTextAppliesEdits(t *testing.T) {
	p := writeFixture(t, []byte("\xEF\xBB\xBFone\r\ntwo\r\n"))
	meta, _, err := ReadText(p, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	nm, err := WriteText(p, "one\ntwo\nthree", meta)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "\xEF\xBB\xBFone\r\ntwo\r\nthree" {
		t.Errorf("got %q", got)
	}
	if nm.FinalNewline || nm.Size != int64(len(got)) || !nm.BOM || nm.EOL != "\r\n" {
		t.Errorf("meta = %+v", nm)
	}
}

func TestReadTextBinary(t *testing.T) {
	p := writeFixture(t, []byte("abc\x00def"))
	meta, text, err := ReadText(p, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Binary || text != "" {
		t.Errorf("meta = %+v text = %q", meta, text)
	}
	if _, err := WriteText(p, "x", meta); !errors.Is(err, ErrNotWritable) {
		t.Errorf("write binary err = %v", err)
	}
}

func TestReadTextNULAfterSniffWindowIsText(t *testing.T) {
	data := append(bytes.Repeat([]byte("a"), binarySniffBytes), 0)
	meta, _, err := ReadText(writeFixture(t, data), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Binary {
		t.Error("NUL beyond 8 KB should not mark binary")
	}
}

func TestReadTextModes(t *testing.T) {
	lim := Limits{HighlightBytes: 10, OpenBytes: 20}
	tests := []struct {
		size int
		want OpenMode
	}{
		{10, ModeNormal},
		{11, ModeNoHighlight},
		{20, ModeNoHighlight},
		{21, ModeReadOnly},
	}
	for _, tt := range tests {
		p := writeFixture(t, bytes.Repeat([]byte("x"), tt.size))
		meta, _, err := ReadText(p, lim)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Mode != tt.want {
			t.Errorf("size %d: mode = %s, want %s", tt.size, meta.Mode, tt.want)
		}
	}
}

func TestWriteTextRefusesReadOnly(t *testing.T) {
	p := writeFixture(t, []byte("orig"))
	_, err := WriteText(p, "new", FileMeta{EOL: "\n", Mode: ModeReadOnly})
	if !errors.Is(err, ErrNotWritable) {
		t.Fatalf("err = %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "orig" {
		t.Errorf("file modified: %q", got)
	}
}

func TestWriteTextNewFileAndNoTempLeft(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "new.go")
	meta, err := WriteText(p, "package x\n", FileMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.EOL != "\n" || !meta.FinalNewline {
		t.Errorf("meta = %+v", meta)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(p)
		if st.Mode().Perm() != 0o644 {
			t.Errorf("new file mode = %v", st.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteTextPreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	p := writeFixture(t, []byte("#!/bin/sh\n"))
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, text, _ := ReadText(p, DefaultLimits)
	if _, err := WriteText(p, text+"echo hi\n", meta); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", st.Mode().Perm())
	}
}

func TestWriteTextFailureCleansUp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	// Renaming a file over a non-empty directory fails on every OS.
	if err := os.WriteFile(filepath.Join(target, "keep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteText(target, "x", FileMeta{}); err == nil {
		t.Fatal("expected error")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestReadTextMissing(t *testing.T) {
	if _, _, err := ReadText(filepath.Join(t.TempDir(), "nope"), DefaultLimits); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v", err)
	}
}
