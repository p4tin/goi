//go:build darwin && !server

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#include <stdlib.h>
#include "dockmenu_darwin.h"

extern void goiDockSelected(long long tag);

@interface GoiDockTarget : NSObject
@end

@implementation GoiDockTarget
- (void)pick:(NSMenuItem *)item {
	goiDockSelected((long long)item.tag);
}
@end

// Touched only on the main thread.
static NSMenu *goiDockMenu;
static GoiDockTarget *goiDockTarget;

static NSMenu *goiApplicationDockMenu(id self, SEL _cmd, NSApplication *app) {
	return goiDockMenu;
}

static int goiDockInstallMain(void) {
	id delegate = [NSApp delegate];
	if (delegate == nil) return 1;
	Class cls = object_getClass(delegate);
	SEL sel = @selector(applicationDockMenu:);
	if (class_getInstanceMethod(cls, sel) != NULL) {
		// Installed by us already (a second call) is fine.
		return method_getImplementation(class_getInstanceMethod(cls, sel)) == (IMP)goiApplicationDockMenu ? 0 : 2;
	}
	return class_addMethod(cls, sel, (IMP)goiApplicationDockMenu, "@@:@") ? 0 : 3;
}

int goiDockInstall(void) {
	if ([NSThread isMainThread]) return goiDockInstallMain();
	__block int r = 0;
	dispatch_sync(dispatch_get_main_queue(), ^{ r = goiDockInstallMain(); });
	return r;
}

void goiDockApply(int n, GoiDockItem *items) {
	NSMutableArray *titles = [NSMutableArray arrayWithCapacity:n];
	int *parents = n ? malloc(sizeof(int) * n) : NULL;
	long long *tags = n ? malloc(sizeof(long long) * n) : NULL;
	int *flags = n ? malloc(sizeof(int) * n) : NULL;
	for (int i = 0; i < n; i++) {
		[titles addObject:[NSString stringWithUTF8String:items[i].title ? items[i].title : ""]];
		parents[i] = items[i].parent;
		tags[i] = items[i].tag;
		flags[i] = items[i].flags;
		free(items[i].title);
	}
	free(items);
	dispatch_async(dispatch_get_main_queue(), ^{
		if (goiDockTarget == nil) goiDockTarget = [GoiDockTarget new];
		NSMenu *root = [[NSMenu alloc] init];
		root.autoenablesItems = NO;
		NSMutableArray *built = [NSMutableArray arrayWithCapacity:n];
		for (int i = 0; i < n; i++) {
			NSMenu *into = root;
			if (parents[i] >= 0 && parents[i] < i) {
				NSMenuItem *owner = built[parents[i]];
				if (owner.submenu) into = owner.submenu;
			}
			NSMenuItem *item;
			if (flags[i] & GOI_DOCK_SEPARATOR) {
				item = [NSMenuItem separatorItem];
			} else {
				item = [[NSMenuItem alloc] initWithTitle:titles[i] action:@selector(pick:) keyEquivalent:@""];
				item.target = goiDockTarget;
				item.tag = (NSInteger)tags[i];
				item.state = (flags[i] & GOI_DOCK_CHECKED) ? NSControlStateValueOn : NSControlStateValueOff;
				item.enabled = !(flags[i] & GOI_DOCK_DISABLED);
				if (flags[i] & GOI_DOCK_SUBMENU) {
					NSMenu *sub = [[NSMenu alloc] initWithTitle:titles[i]];
					sub.autoenablesItems = NO;
					item.submenu = sub;
					item.action = nil;
				}
			}
			[into addItem:item];
			[built addObject:item];
		}
		goiDockMenu = n ? root : nil;
		free(parents);
		free(tags);
		free(flags);
	});
}
