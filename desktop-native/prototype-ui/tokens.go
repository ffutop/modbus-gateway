// PROTOTYPE — shared token adapter; rebuild to pick up design/tokens.json changes.
package main

import (
	"fmt"
	"gioui.org/unit"
	"github.com/ffutop/modbus-gateway/design"
	"image/color"
	"math"
	"strconv"
)

var prototypeTokens = func() *design.Tokens {
	t, err := design.Load()
	if err != nil {
		panic(err)
	}
	return t
}()

func tokenColor(name string) color.NRGBA {
	hex, ok := prototypeTokens.Hex(name)
	if !ok {
		panic("missing color token: " + name)
	}
	n, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		panic(err)
	}
	return rgb(uint32(n))
}
func tokenSize(name string) unit.Sp {
	for _, d := range prototypeTokens.FontSize {
		if d.Name == name {
			return unit.Sp(math.Round(d.Value*prototypeTokens.Density.DesktopNative*2) / 2)
		}
	}
	panic("missing font size token: " + name)
}
func tokenRadius(name string) unit.Dp {
	for _, d := range prototypeTokens.Radius {
		if d.Name == name {
			return unit.Dp(d.Value)
		}
	}
	panic("missing radius token: " + name)
}
func tokenAlpha(name string) color.NRGBA {
	for _, v := range prototypeTokens.Alpha {
		if v.Name == name {
			var r, g, b int
			var a float64
			if _, err := fmt.Sscanf(v.Value, "rgba(%d,%d,%d,%f)", &r, &g, &b, &a); err != nil {
				panic(err)
			}
			return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(math.Round(a * 255))}
		}
	}
	panic("missing alpha token: " + name)
}

var (
	colInk        = tokenColor("ink")
	colBody       = tokenColor("body")
	colMuted      = tokenColor("muted")
	colFaint      = tokenColor("faint")
	colCanvas     = tokenColor("canvas")
	colSoft       = tokenColor("soft")
	colCard       = tokenColor("card")
	colHair       = tokenColor("hair")
	colHairSoft   = tokenColor("hair-soft")
	colOk         = tokenColor("ok")
	colOkBg       = tokenColor("ok-bg")
	colOkSolid    = tokenColor("ok-solid")
	colWarn       = tokenColor("warn")
	colWarnBg     = tokenColor("warn-bg")
	colWarnSolid  = tokenColor("warn-solid")
	colErr        = tokenColor("err")
	colErrBg      = tokenColor("err-bg")
	colErrTint    = tokenColor("err-tint")
	colErrSolid   = tokenColor("err-solid")
	colAccent     = tokenColor("accent")
	colAccentBg   = tokenColor("accent-bg")
	colAccentMd   = tokenColor("accent-md")
	colFlash      = tokenColor("highlight")
	colDark       = tokenColor("dark")
	colHover      = tokenColor("hover")
	colLine       = tokenColor("line")
	colLineStrong = tokenColor("line-strong")
	colOnDark     = tokenColor("on-dark")
	colOnDarkBody = tokenColor("on-dark-body")
	textSize      = tokenSize("body")
	smallSize     = tokenSize("caption")
	monoSize      = tokenSize("small")
	microSize     = tokenSize("micro")
	titleSize     = tokenSize("title")
	radiusXs      = tokenRadius("xs")
	radiusSm      = tokenRadius("sm")
	radiusMd      = tokenRadius("md")
	radiusLg      = tokenRadius("lg")
	radiusPill    = tokenRadius("pill")
	colScrim      = tokenAlpha("scrim")
)
