package rpc

import "github.com/p4tin/goi/internal/pack"

// RunMarkers returns the runnable spots of a buffer for the gutter (Goi:
// main, tests, benchmarks, fuzz targets, examples). Files the pack doesn't
// run return none.
func (s *EditorService) RunMarkers(path, content string) []pack.RunMarker {
	r := s.app.pack.Run
	if r == nil {
		return []pack.RunMarker{}
	}
	m := r.Markers(path, content)
	if m == nil {
		m = []pack.RunMarker{}
	}
	return m
}
