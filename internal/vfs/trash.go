package vfs

import "errors"

// ErrTrashUnavailable means the OS trash cannot take the item; callers should
// ask the user before deleting permanently.
var ErrTrashUnavailable = errors.New("vfs: trash unavailable")

// Trash moves path to the OS trash / recycle bin.
func Trash(path string) error {
	return trash(path)
}

// Reveal shows path in the OS file manager.
func Reveal(path string) error {
	return reveal(path)
}
