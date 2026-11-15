package services

import "testing"

func TestGoMajorMinor(t *testing.T) {
	for in, want := range map[string]string{"go1.26.0": "1.26", "go1.27rc1": "1.27", "go1.25": "1.25", "devel abc": "1.26"} {
		if got := goMajorMinor(in); got != want {
			t.Errorf("goMajorMinor(%q) = %q, want %q", in, got, want)
		}
	}
}
