//go:build !darwin || server

package dockmenu

// Install is a no-op: only macOS has a Dock menu.
func Install() error { return nil }

func apply(gen uint32, items []flatItem) {}
