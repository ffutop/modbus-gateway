// Package filepicker shows the platform's own file chooser for opening a
// configuration file or saving it under a new name: NSOpenPanel and
// NSSavePanel on macOS, the common file dialogs on Windows, and the XDG
// desktop portal (or zenity / kdialog) on Linux. All of them ship with the
// operating system or desktop, so the app gains no runtime dependency; where
// none is available Pick returns ErrUnavailable and the caller falls back to
// its own dialog.
package filepicker

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
)

// ErrUnavailable means the platform offers no file chooser.
var ErrUnavailable = errors.New("no native file chooser")

// Request describes one chooser. Save asks for a new or existing path to
// write and has the chooser confirm replacing an existing file.
type Request struct {
	Save  bool
	Title string
	Dir   string // initial directory
	Name  string // suggested file name when saving
}

// Pick shows the chooser and blocks until the user is done; it returns ""
// when they cancelled. Call it off the UI goroutine.
func Pick(r Request) (string, error) {
	if r.Title == "" {
		r.Title = "打开配置文件"
		if r.Save {
			r.Title = "另存为"
		}
	}
	return pick(r)
}

// owner is the platform handle of the app window, when the platform needs it
// to make the chooser modal (Windows).
var owner struct {
	sync.Mutex
	handle uintptr
}

func setOwner(h uintptr)   { owner.Lock(); owner.handle = h; owner.Unlock() }
func ownerHandle() uintptr { owner.Lock(); defer owner.Unlock(); return owner.handle }

// Patterns are the file name patterns choosers filter by.
var Patterns = []string{"*.yaml", "*.yml"}

// IsYAML reports whether path has a configuration file extension.
func IsYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}
