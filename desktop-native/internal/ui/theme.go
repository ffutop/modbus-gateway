// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package ui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/uifont"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xFF}
}

// Colors, font sizes and radii are generated from design/tokens.json into
// tokens_gen.go; edit the tokens, not these files.
const (
	textSize  = fsBody
	smallSize = fsCaption
	monoSize  = fsSmall
)

var monoFont = font.Font{Typeface: uifont.Mono}

// Theme is the app-wide material theme. Latin text uses the bundled Go
// fonts; CJK falls back to bundled Noto Sans SC without system font discovery.
type Theme struct {
	*material.Theme
}

func NewTheme() *Theme {
	th := material.NewTheme()
	th.Shaper = uifont.NewShaper()
	th.Face = uifont.UI
	th.Palette = material.Palette{Bg: colCanvas, Fg: colBody, ContrastBg: colInk, ContrastFg: colCanvas}
	th.TextSize = textSize
	return &Theme{th}
}

// label is a single-line label, truncated with an ellipsis.
func (th *Theme) label(txt string, size unit.Sp, col color.NRGBA) material.LabelStyle {
	l := material.Label(th.Theme, size, txt)
	l.Color = col
	l.MaxLines = 1
	return l
}

func (th *Theme) mono(txt string, col color.NRGBA) material.LabelStyle {
	l := th.label(txt, monoSize, col)
	l.Font = monoFont
	return l
}

func (th *Theme) bold(txt string, size unit.Sp, col color.NRGBA) material.LabelStyle {
	l := th.label(txt, size, col)
	l.Font.Weight = font.SemiBold
	return l
}

// fill paints the area of gtx.Constraints.Min.
func fill(gtx C, col color.NRGBA) D {
	defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, col)
	return D{Size: gtx.Constraints.Min}
}

// background lays out w over a filled rectangle of its own size.
func background(gtx C, col color.NRGBA, w layout.Widget) D {
	return layout.Background{}.Layout(gtx, func(gtx C) D { return fill(gtx, col) }, w)
}

// rounded is background with rounded corners. A radius beyond half the
// shorter side (e.g. radiusPill) gives a pill.
func rounded(gtx C, col color.NRGBA, radius unit.Dp, w layout.Widget) D {
	return layout.Background{}.Layout(gtx, func(gtx C) D {
		size := gtx.Constraints.Min
		r := min(gtx.Dp(radius), size.X/2, size.Y/2)
		defer clip.UniformRRect(image.Rectangle{Max: size}, r).Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, col)
		return D{Size: gtx.Constraints.Min}
	}, w)
}

// hline is a 1px horizontal rule across the available width.
func hline(gtx C, col color.NRGBA) D {
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 1)
	return fill(gtx, col)
}

// vline is a 1px vertical rule across the available height.
func vline(gtx C, col color.NRGBA) D {
	gtx.Constraints.Min = image.Pt(1, gtx.Constraints.Max.Y)
	return fill(gtx, col)
}

// fixed lays out w in exactly width dp.
func fixed(gtx C, width unit.Dp, w layout.Widget) D {
	px := gtx.Dp(width)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
	d := w(gtx)
	d.Size.X = px
	return d
}

// row lays out children left to right in a band of height dp spanning the
// available width, vertically centered, children sharing one baseline.
func row(gtx C, height unit.Dp, children ...layout.FlexChild) D {
	h := gtx.Dp(height)
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, h)
	gtx.Constraints.Max.Y = h
	return layout.W.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 0)
		return layout.Flex{Alignment: layout.Baseline}.Layout(gtx, children...)
	})
}

// badge is a small pill with text.
func (th *Theme) badge(gtx C, txt string, fg, bg color.NRGBA) D {
	return rounded(gtx, bg, radiusPill, func(gtx C) D {
		return layout.Inset{Left: 7, Right: 7, Top: 1, Bottom: 1}.Layout(gtx, th.label(txt, smallSize, fg).Layout)
	})
}

// gap is horizontal space between flex children.
func gap(dp unit.Dp) layout.FlexChild {
	return layout.Rigid(layout.Spacer{Width: dp}.Layout)
}
