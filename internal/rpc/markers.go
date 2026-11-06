package rpc

import (
	"strings"

	"github.com/p4tin/goi/internal/gosyntax"
)

// RunMarkers returns runnable functions (main, tests, benchmarks, fuzz
// targets, examples) in a Go buffer. Non-Go paths return none.
func (s *EditorService) RunMarkers(path, content string) []gosyntax.RunMarker {
	if !strings.HasSuffix(path, ".go") {
		return []gosyntax.RunMarker{}
	}
	m := gosyntax.RunMarkers(path, content)
	if m == nil {
		m = []gosyntax.RunMarker{}
	}
	return m
}
