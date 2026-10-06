//go:build !windows

package filepicker

import "gioui.org/app"

// macOS panels are application-modal on their own; the portal and the dialog
// tools place their windows themselves.
func viewHandle(app.ViewEvent) uintptr { return 0 }
