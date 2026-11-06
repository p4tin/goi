package vfs

import "testing"

func TestMatcher(t *testing.T) {
	m, err := NewMatcher([]string{".git", "node_modules", "*.log", ".goi/cache", "build/**", "tmp/"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{".git", true, true},
		{"sub/.git", true, true},
		{"sub/.git/HEAD", false, true},
		{"node_modules/x/y.js", false, true},
		{"a/b/c.log", false, true},
		{"a/b/c.go", false, false},
		{".goi/cache", true, true},
		{".goi/cache/x", false, true},
		{".goi/settings.toml", false, false},
		{"sub/.goi/cache", true, false}, // anchored
		{"build/bin/goi", false, true},
		{"src/build/x", false, false}, // anchored
		{"tmp", true, true},
		{"tmp", false, false}, // dir-only
		{"tmp/a.txt", false, true},
		{"", true, false},
	}
	for _, tt := range tests {
		if got := m.Match(tt.rel, tt.isDir); got != tt.want {
			t.Errorf("Match(%q, %v) = %v, want %v", tt.rel, tt.isDir, got, tt.want)
		}
	}
}

func TestMatcherInvalidAndEmpty(t *testing.T) {
	if _, err := NewMatcher([]string{"a[b"}); err == nil {
		t.Error("expected error for invalid pattern")
	}
	m, err := NewMatcher([]string{"", "  "})
	if err != nil {
		t.Fatal(err)
	}
	if m.Match("anything", false) {
		t.Error("empty matcher should match nothing")
	}
}
