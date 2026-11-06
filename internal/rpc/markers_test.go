package rpc

import "testing"

func TestRunMarkersService(t *testing.T) {
	s := &EditorService{}
	got := s.RunMarkers("/p/x_test.go", "package x\nimport \"testing\"\nfunc TestA(t *testing.T) {}\n")
	if len(got) != 1 || got[0].Name != "TestA" || got[0].Line != 3 {
		t.Errorf("got %+v", got)
	}
	if got := s.RunMarkers("/p/notes.txt", "func main() {}"); len(got) != 0 {
		t.Errorf("non-Go file: %+v", got)
	}
}
