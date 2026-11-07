//go:build !darwin

package appearance

// Set is a no-op outside macOS.
func Set(Mode) {}
