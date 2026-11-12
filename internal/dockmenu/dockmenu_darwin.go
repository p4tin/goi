//go:build darwin && !server

package dockmenu

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "dockmenu_darwin.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Install adds the Dock menu to the running application's delegate. Call it
// once the app has started (its delegate exists). It refuses to replace a
// Dock menu the delegate already provides.
func Install() error {
	switch C.goiDockInstall() {
	case 0:
		return nil
	case 1:
		return errors.New("dockmenu: application has no delegate yet")
	case 2:
		return errors.New("dockmenu: delegate already implements applicationDockMenu:")
	default:
		return errors.New("dockmenu: could not add applicationDockMenu:")
	}
}

func apply(gen uint32, items []flatItem) {
	n := len(items)
	if n == 0 {
		C.goiDockApply(0, nil)
		return
	}
	cItems := (*[1 << 20]C.GoiDockItem)(C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(C.GoiDockItem{}))))[:n:n]
	for i, it := range items {
		var flags C.int
		if it.Checked {
			flags |= C.GOI_DOCK_CHECKED
		}
		if it.Disabled {
			flags |= C.GOI_DOCK_DISABLED
		}
		if it.Separator {
			flags |= C.GOI_DOCK_SEPARATOR
		}
		if it.Submenu {
			flags |= C.GOI_DOCK_SUBMENU
		}
		cItems[i] = C.GoiDockItem{
			parent: C.int(it.Parent),
			tag:    C.longlong(tag(gen, i)),
			flags:  flags,
			title:  C.CString(it.Title),
		}
	}
	// goiDockApply copies the items and frees the titles and the array.
	C.goiDockApply(C.int(n), &cItems[0])
}

//export goiDockSelected
func goiDockSelected(t C.longlong) { selected(int64(t)) }
