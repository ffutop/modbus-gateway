// Package uifont supplies the desktop UI's bundled, platform-independent fonts.
package uifont

import (
	_ "embed"
	"sync"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/text"
)

const (
	UI   font.Typeface = "Noto Sans SC"
	Mono font.Typeface = "Go Mono, Noto Sans SC"
)

//go:embed assets/NotoSansSC-Regular.otf
var regular []byte

//go:embed assets/NotoSansSC-Bold.otf
var bold []byte

var once sync.Once
var collection []font.FontFace

// Collection returns Go's faces followed by the bundled Simplified Chinese
// regular and bold faces. The caller receives its own slice.
func Collection() []font.FontFace {
	once.Do(func() {
		collection = append([]font.FontFace(nil), gofont.Collection()...)
		for _, data := range [][]byte{regular, bold} {
			faces, err := opentype.ParseCollection(data)
			if err != nil {
				panic("bundled Noto Sans SC: " + err.Error())
			}
			collection = append(collection, faces...)
		}
	})
	return append([]font.FontFace(nil), collection...)
}

// NewShaper deliberately disables system font discovery: missing glyphs use
// the same bundled faces on every machine, including headless snapshots.
func NewShaper() *text.Shaper {
	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(Collection()))
}
