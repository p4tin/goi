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
	if got, ok := s.OpenSet(); got != nil || ok {
		t.Fatalf("initial open set = %v, %v; want nil, false", got, ok)
	}
	want := []string{"/p/a", "/p/b"}
	if err := s.SetOpenSet(want); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.OpenSet(); !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("open set = %v, want %v", got, want)
	}
	if err := s.SetOpenSet(nil); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.OpenSet(); !ok || len(got) != 0 {
		t.Fatalf("cleared open set = %v, %v; want empty and saved", got, ok)
	}
	// The recent list itself is untouched.
	if list, err := s.List(); err != nil || len(list) != 0 {
		t.Fatalf("list = %v, %v", list, err)
	}
}
