//go:build darwin

package appearance

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// mode: 0 system, 1 light, 2 dark.
static void goiSetAppearance(int mode) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSAppearance *a = nil;
		if (mode == 1) a = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
		if (mode == 2) a = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
		[NSApp setAppearance:a];
	});
}
*/
import "C"

// Set applies m to the whole application. Safe to call from any goroutine.
func Set(m Mode) {
	switch m {
	case Light:
		C.goiSetAppearance(1)
	case Dark:
		C.goiSetAppearance(2)
	default:
		C.goiSetAppearance(0)
	}
}
