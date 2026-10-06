package filepicker

import "gioui.org/app"

// The window owns the dialog, so the dialog is modal to it.
func viewHandle(e app.ViewEvent) uintptr {
	if v, ok := e.(app.Win32ViewEvent); ok {
		return v.HWND
	}
	return 0
}
