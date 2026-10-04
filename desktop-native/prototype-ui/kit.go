// PROTOTYPE — throwaway. Shared drawing helpers for the UI variants.
// Copied from internal/ui on purpose: variants must be free to diverge.

package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/uifont"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/decode"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xFF}
}

func alpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

var monoFont = font.Font{Typeface: uifont.Mono}

type Theme struct{ *material.Theme }

func NewTheme() *Theme {
	th := material.NewTheme()
	th.Shaper = uifont.NewShaper()
	th.Face = uifont.UI
	th.Palette = material.Palette{Bg: colCanvas, Fg: colBody, ContrastBg: colInk, ContrastFg: colCanvas}
	th.TextSize = textSize
	return &Theme{th}
}

// LabelStyle preserves the material API while giving single-line UI text a
// stable line box and baseline independent of the string's fallback glyphs.
type LabelStyle struct{ material.LabelStyle }

func uiLineHeight(size unit.Sp) unit.Sp { return unit.Sp(math.Ceil(float64(size) * 1.6)) }

func (l LabelStyle) Layout(gtx C) D {
	if l.MaxLines != 1 {
		return l.LabelStyle.Layout(gtx)
	}
	minimum := gtx.Constraints.Min
	gtx.Constraints.Min.Y = 0
	macro := op.Record(gtx.Ops)
	d := l.LabelStyle.Layout(gtx)
	call := macro.Stop()
	height := max(gtx.Sp(uiLineHeight(l.TextSize)), minimum.Y)
	height = min(height, gtx.Constraints.Max.Y)
	baseline := gtx.Sp(unit.Sp(math.Ceil(float64(l.TextSize) * 1.15)))
	baseline += max(0, height-gtx.Sp(uiLineHeight(l.TextSize))) / 2
	offset := baseline - (d.Size.Y - d.Baseline)
	translated := op.Offset(image.Pt(0, offset)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	translated.Pop()
	d.Size.Y, d.Baseline = height, height-baseline
	return d
}

func (th *Theme) label(txt string, size unit.Sp, col color.NRGBA) LabelStyle {
	l := LabelStyle{LabelStyle: material.Label(th.Theme, size, txt)}
	l.LineHeight = uiLineHeight(size)
	l.LineHeightScale = 1
	l.Color = col
	l.MaxLines = 1
	return l
}

func (th *Theme) mono(txt string, col color.NRGBA) LabelStyle {
	l := th.label(txt, monoSize, col)
	l.Font = monoFont
	return l
}

func (th *Theme) bold(txt string, size unit.Sp, col color.NRGBA) LabelStyle {
	l := th.label(txt, size, col)
	l.Font.Weight = font.SemiBold
	return l
}

func fill(gtx C, col color.NRGBA) D {
	defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, col)
	return D{Size: gtx.Constraints.Min}
}

func background(gtx C, col color.NRGBA, w layout.Widget) D {
	return layout.Background{}.Layout(gtx, func(gtx C) D { return fill(gtx, col) }, w)
}

func rounded(gtx C, col color.NRGBA, radius unit.Dp, w layout.Widget) D {
	return layout.Background{}.Layout(gtx, func(gtx C) D {
		defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, min(gtx.Dp(radius), min(gtx.Constraints.Min.X, gtx.Constraints.Min.Y)/2)).Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, col)
		return D{Size: gtx.Constraints.Min}
	}, w)
}

// outlined draws a 1px border of col around w.
func outlined(gtx C, col, bg color.NRGBA, radius unit.Dp, w layout.Widget) D {
	return rounded(gtx, col, radius, func(gtx C) D {
		return layout.UniformInset(1).Layout(gtx, func(gtx C) D {
			return rounded(gtx, bg, radius-1, w)
		})
	})
}

func hline(gtx C, col color.NRGBA) D {
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 1)
	return fill(gtx, col)
}

func vline(gtx C, col color.NRGBA) D {
	gtx.Constraints.Min = image.Pt(1, gtx.Constraints.Max.Y)
	return fill(gtx, col)
}

func fixed(gtx C, width unit.Dp, w layout.Widget) D {
	px := gtx.Dp(width)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
	d := w(gtx)
	d.Size.X = px
	return d
}

func fixedH(gtx C, height unit.Dp, w layout.Widget) D {
	h := gtx.Dp(height)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = 0, h
	macro := op.Record(gtx.Ops)
	d := w(gtx)
	call := macro.Stop()
	offset := max(0, h-d.Size.Y) / 2
	translated := op.Offset(image.Pt(0, offset)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	translated.Pop()
	d.Baseline += h - d.Size.Y - offset
	d.Size.Y = h
	return d
}

func row(gtx C, height unit.Dp, children ...layout.FlexChild) D {
	h := gtx.Dp(height)
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, h)
	gtx.Constraints.Max.Y = h
	return layout.W.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 0)
		return layout.Flex{Alignment: layout.Baseline}.Layout(gtx, children...)
	})
}

func gap(dp unit.Dp) layout.FlexChild  { return layout.Rigid(layout.Spacer{Width: dp}.Layout) }
func vgap(dp unit.Dp) layout.FlexChild { return layout.Rigid(layout.Spacer{Height: dp}.Layout) }

func (th *Theme) badge(gtx C, txt string, fg, bg color.NRGBA) D {
	gtx.Constraints.Min.X = 0
	return rounded(gtx, bg, radiusPill, func(gtx C) D {
		return layout.Inset{Left: 7, Right: 7, Top: 1, Bottom: 1}.Layout(gtx, th.label(txt, smallSize, fg).Layout)
	})
}

// disclosure is a drawn icon, avoiding dependency on uncommon font symbols.
func disclosure(gtx C, expanded bool, col color.NRGBA) D {
	h := gtx.Sp(uiLineHeight(smallSize))
	x, y, r := float32(gtx.Dp(4)), float32(h)/2, float32(gtx.Dp(3))
	var p clip.Path
	p.Begin(gtx.Ops)
	if expanded {
		p.MoveTo(f32.Pt(x-r, y-r/2))
		p.LineTo(f32.Pt(x+r, y-r/2))
		p.LineTo(f32.Pt(x, y+r))
	} else {
		p.MoveTo(f32.Pt(x-r/2, y-r))
		p.LineTo(f32.Pt(x+r, y))
		p.LineTo(f32.Pt(x-r/2, y+r))
	}
	p.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
	baseline := gtx.Sp(unit.Sp(math.Ceil(float64(smallSize) * 1.15)))
	return D{Size: image.Pt(gtx.Dp(8), h), Baseline: h - baseline}
}

func dot(gtx C, col color.NRGBA) D {
	s := gtx.Dp(7)
	defer clip.Ellipse{Max: image.Pt(s, s)}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, col)
	return D{Size: image.Pt(s, s), Baseline: 0}
}

// button is an outlined button; primary fills it with ink.
func (th *Theme) button(gtx C, btn *widget.Clickable, txt string, primary bool) D {
	bg, fg, border := colCanvas, colInk, colHair
	if primary {
		bg, fg, border = colInk, colCanvas, colInk
	} else if btn.Hovered() {
		bg = colSoft
	}
	return btn.Layout(gtx, func(gtx C) D {
		return outlined(gtx, border, bg, radiusSm, func(gtx C) D {
			return layout.Inset{Left: 10, Right: 10, Top: 4, Bottom: 4}.Layout(gtx, th.bold(txt, textSize, fg).Layout)
		})
	})
}

// chip is a compact desktop toggle with a persistent control outline.
func (th *Theme) chip(gtx C, btn *widget.Clickable, txt string, on bool) D {
	bg, fg := colCard, colMuted
	if on {
		bg, fg = colInk, colCanvas
	} else if btn.Hovered() {
		bg = colHover
	}
	return btn.Layout(gtx, func(gtx C) D {
		return rounded(gtx, bg, radiusPill, func(gtx C) D {
			return layout.Inset{Left: 10, Right: 10, Top: 3, Bottom: 3}.Layout(gtx, th.label(txt, smallSize, fg).Layout)
		})
	})
}

// tab is an underlined top-level tab.
func (th *Theme) tab(gtx C, btn *widget.Clickable, txt string, on bool) D {
	fg := colMuted
	if on {
		fg = colInk
	}
	return btn.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: 12, Right: 12, Top: 6, Bottom: 6}.Layout(gtx, th.bold(txt, textSize, fg).Layout)
			}),
			layout.Rigid(func(gtx C) D {
				if !on {
					return layout.Spacer{Height: 2}.Layout(gtx)
				}
				gtx.Constraints.Min = image.Pt(gtx.Constraints.Min.X, gtx.Dp(2))
				return fill(gtx, colInk)
			}),
		)
	})
}

// clicks keeps one Clickable per key, so a click stays with its item while
// lists shift.
type clicks[K comparable] map[K]*widget.Clickable

func (c clicks[K]) get(k K) *widget.Clickable {
	b := c[k]
	if b == nil {
		b = new(widget.Clickable)
		c[k] = b
	}
	return b
}

// propRow is a label/value line in a property panel.
func (th *Theme) propRow(gtx C, label, value string) D {
	return layout.Inset{Left: 14, Right: 14, Top: 3, Bottom: 3}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
			layout.Rigid(func(gtx C) D { return fixed(gtx, 84, th.label(label, textSize, colMuted).Layout) }),
			layout.Flexed(1, th.label(value, textSize, colInk).Layout),
		)
	})
}

func (th *Theme) sectionTitle(txt string) layout.Widget {
	return func(gtx C) D {
		return layout.Inset{Left: 14, Right: 14, Top: 12, Bottom: 6}.Layout(gtx, th.bold(txt, smallSize, colMuted).Layout)
	}
}

func durText(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%.2f s", d.Seconds())
	}
	return fmt.Sprintf("%.2f ms", float64(d)/float64(time.Millisecond))
}

func fcText(e *telemetry.Event) string {
	return fmt.Sprintf("%02X %s", e.FunctionCode, decode.FunctionName(e.FunctionCode))
}

func resultText(e *telemetry.Event) (string, color.NRGBA) {
	if e.Err != nil {
		return e.Err.Error(), colErr
	}
	if len(e.Response) > 0 && e.Response[0]&0x80 != 0 {
		return "异常响应", colErr
	}
	return "正常", colOk
}

func failed(e *telemetry.Event) bool {
	return e.Err != nil || (len(e.Response) > 0 && e.Response[0]&0x80 != 0)
}

// fieldsView lays out decoded request and response fields of e.
func (th *Theme) fieldsView(gtx C, list *widget.List, e *telemetry.Event) D {
	type item struct {
		title string
		f     *decode.Field
	}
	var items []item
	req := decode.Request(e.Request)
	items = append(items, item{title: fmt.Sprintf("请求 · %d 字节", len(e.Request))})
	for i := range req {
		items = append(items, item{f: &req[i]})
	}
	if e.Err != nil {
		items = append(items, item{title: "响应 · 无（" + e.Err.Error() + "）"})
	} else {
		resp := decode.Response(e.Response, e.Request)
		items = append(items, item{title: fmt.Sprintf("响应 · %d 字节", len(e.Response))})
		for i := range resp {
			items = append(items, item{f: &resp[i]})
		}
	}
	return material.List(th.Theme, list).Layout(gtx, len(items), func(gtx C, i int) D {
		it := items[i]
		if it.f == nil {
			return layout.Inset{Left: 14, Top: 6, Bottom: 2}.Layout(gtx, th.bold(it.title, textSize, colInk).Layout)
		}
		f := it.f
		vc := colInk
		if f.Err {
			vc = colErr
		}
		return layout.Inset{Left: 14 + unit.Dp(f.Depth)*16, Right: 14}.Layout(gtx, func(gtx C) D {
			return row(gtx, 21,
				layout.Rigid(func(gtx C) D { return fixed(gtx, 88, th.label(f.Label, textSize, colMuted).Layout) }),
				layout.Flexed(1, th.label(f.Value, textSize, vc).Layout),
			)
		})
	})
}

// hexDump lays out pdu as offset / hex / ASCII rows.
func (th *Theme) hexDump(gtx C, title string, pdu []byte) D {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14, Top: 6, Bottom: 2}.Layout(gtx, th.bold(title, textSize, colInk).Layout)
		}),
	}
	for lo := 0; lo < len(pdu); lo += 16 {
		hi := min(lo+16, len(pdu))
		hex, ascii := "", ""
		for i := lo; i < lo+16; i++ {
			if i == lo+8 {
				hex += " "
			}
			if i < hi {
				hex += fmt.Sprintf("%02X ", pdu[i])
				if pdu[i] >= 0x20 && pdu[i] < 0x7F {
					ascii += string(rune(pdu[i]))
				} else {
					ascii += "."
				}
			} else {
				hex += "   "
			}
		}
		children = append(children, layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14}.Layout(gtx, func(gtx C) D {
				return row(gtx, 19,
					layout.Rigid(func(gtx C) D { return fixed(gtx, 44, th.mono(fmt.Sprintf("%04X", lo), colFaint).Layout) }),
					layout.Rigid(th.mono(hex, colBody).Layout),
					gap(10),
					layout.Rigid(th.mono(ascii, colMuted).Layout),
				)
			})
		}))
	}
	if len(pdu) == 0 {
		children = append(children, layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14}.Layout(gtx, th.label("无数据", smallSize, colFaint).Layout)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}
