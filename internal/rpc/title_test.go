package rpc

import (
	"path/filepath"
	"testing"
)

func TestWindowTitleFollowsProject(t *testing.T) {
	a, _ := started(t, "")
	var titles []string
	a.mainWindow().handle = &fakeHandle{title: func(s string) { titles = append(titles, s) }}
	root := module(t)

	if err := a.openProject(root); err != nil {
		t.Fatal(err)
	}
	if err := a.closeProject(); err != nil {
		t.Fatal(err)
	}
	want := []string{"Goi - " + filepath.Base(root), "Goi"}
	if len(titles) != 2 || titles[0] != want[0] || titles[1] != want[1] {
		t.Errorf("titles = %q, want %q", titles, want)
	}
}
