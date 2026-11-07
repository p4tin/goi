package rpc

import (
	"testing"

	"github.com/p4tin/goi/internal/appearance"
)

func TestSetAppearanceMapsTheme(t *testing.T) {
	a, _ := started(t, "")
	var got []appearance.Mode
	old := setAppearance
	t.Cleanup(func() { setAppearance = old })
	setAppearance = func(m appearance.Mode) { got = append(got, m) }

	for _, theme := range []string{"auto", "light", "dark", "hc", "bogus"} {
		a.appSvc.SetAppearance(theme)
	}
	want := []appearance.Mode{appearance.System, appearance.Light, appearance.Dark, appearance.Dark, appearance.System}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("theme %d: got %v, want %v", i, got[i], want[i])
		}
	}
}
