package vfs

import (
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	foDelete          = 0x0003
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040
	fofNoErrorUI      = 0x0400
)

// shFileOpStructW mirrors SHFILEOPSTRUCTW.
type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

var procSHFileOperationW = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

func trash(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("vfs: trash: %w", err)
	}
	from, err := windows.UTF16FromString(abs)
	if err != nil {
		return fmt.Errorf("vfs: trash: %w", err)
	}
	from = append(from, 0) // pFrom is double-NUL terminated
	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofNoErrorUI | fofSilent,
	}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 {
		return fmt.Errorf("vfs: trash %s: SHFileOperation code 0x%x", abs, r)
	}
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("vfs: trash %s: aborted", abs)
	}
	return nil
}
