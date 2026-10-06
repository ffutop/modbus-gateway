package filepicker

import (
	"gioui.org/app"
	"gioui.org/io/event"
)

// Observe records the window handle from the window's events; pass it every
// event from Window.Event.
func Observe(e event.Event) {
	if v, ok := e.(app.ViewEvent); ok {
		setOwner(viewHandle(v))
	}
}
