// Package appearance sets the native light/dark appearance of the app's
// window chrome (title bar, menus, sheets) independently of the OS setting.
package appearance

// Mode is the native appearance to use.
type Mode string

const (
	System Mode = "system" // follow the OS appearance
	Light  Mode = "light"
	Dark   Mode = "dark"
)
