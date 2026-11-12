package recent

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenSetRoundTrip(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "recent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.OpenSet(); got != nil {
		t.Fatalf("initial open set = %v, want nil", got)
	}
	want := []string{"/p/a", "/p/b"}
	if err := s.SetOpenSet(want); err != nil {
		t.Fatal(err)
	}
	if got := s.OpenSet(); !reflect.DeepEqual(got, want) {
		t.Fatalf("open set = %v, want %v", got, want)
	}
	if err := s.SetOpenSet(nil); err != nil {
		t.Fatal(err)
	}
	if got := s.OpenSet(); len(got) != 0 {
		t.Fatalf("cleared open set = %v", got)
	}
	// The recent list itself is untouched.
	if list, err := s.List(); err != nil || len(list) != 0 {
		t.Fatalf("list = %v, %v", list, err)
	}
}
