package filepicker

/*
#cgo CFLAGS: -x objective-c -Wno-deprecated-declarations
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>
#include <stdlib.h>

// pick runs the panel on the main thread, where AppKit requires it, and
// returns the chosen path (malloc'ed) or NULL if cancelled.
static char *pick(int save, const char *title, const char *dir, const char *name) {
	__block char *result = NULL;
	NSString *t = [NSString stringWithUTF8String:title];
	NSString *d = [NSString stringWithUTF8String:dir];
	NSString *n = [NSString stringWithUTF8String:name];
	void (^run)(void) = ^{
		@autoreleasepool {
			NSSavePanel *panel;
			if (save) {
				panel = [NSSavePanel savePanel];
				[panel setNameFieldStringValue:n];
				[panel setCanCreateDirectories:YES];
			} else {
				NSOpenPanel *open = [NSOpenPanel openPanel];
				[open setCanChooseFiles:YES];
				[open setCanChooseDirectories:NO];
				[open setAllowsMultipleSelection:NO];
				panel = open;
			}
			// The first type is appended to a save name without extension.
			[panel setAllowedFileTypes:@[@"yaml", @"yml"]];
			[panel setTitle:t];
			if ([d length] > 0) {
				[panel setDirectoryURL:[NSURL fileURLWithPath:d isDirectory:YES]];
			}
			[NSApp activateIgnoringOtherApps:YES];
			if ([panel runModal] == NSModalResponseOK) {
				result = strdup([[[panel URL] path] fileSystemRepresentation]);
			}
		}
	};
	if ([NSThread isMainThread]) {
		run();
	} else {
		dispatch_sync(dispatch_get_main_queue(), run);
	}
	return result;
}
*/
import "C"

import (
	"unsafe"
)

func pick(r Request) (string, error) {
	title, dir, name := C.CString(r.Title), C.CString(r.Dir), C.CString(r.Name)
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(dir))
	defer C.free(unsafe.Pointer(name))
	save := C.int(0)
	if r.Save {
		save = 1
	}
	path := C.pick(save, title, dir, name)
	if path == nil {
		return "", nil
	}
	defer C.free(unsafe.Pointer(path))
	return C.GoString(path), nil
}
