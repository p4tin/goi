//go:build !darwin && !windows && !linux

package vfs

func trash(string) error { return ErrTrashUnavailable }
