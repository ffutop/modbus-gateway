// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package ui

import (
	"fmt"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/decode"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

const bytesPerRow = 16

// fieldRef names a field: which PDU, and its index in that PDU's fields.
type fieldRef struct {
	pdu   int // 0 = request, 1 = response
	field int
}

var noField = fieldRef{-1, -1}

// pduView is one decoded PDU with the clickables of its fields and bytes.
type pduView struct {
	title  string
	data   []byte
	fields []decode.Field
	note   string // shown instead of fields when there is no PDU
	rows   []widget.Clickable
	bytes  []widget.Clickable // hex cells
	chars  []widget.Clickable // ASCII cells
}

// inspector shows the selected request: decoded fields on the left, raw
// bytes on the right. Selecting or hovering either side highlights the
// other.
type inspector struct {
	seq      uint64
	pdus     [2]pduView
	selected fieldRef
	hovered  fieldRef

	fieldList widget.List
	hexList   widget.List
}

func (in *inspector) init() {
	in.fieldList.Axis = layout.Vertical
	in.hexList.Axis = layout.Vertical
	in.selected, in.hovered = noField, noField
}

// load rebuilds the views when the selection changes.
func (in *inspector) load(e *telemetry.Event) {
	if e.Seq == in.seq && in.pdus[0].title != "" {
		return
	}
	in.seq = e.Seq
	in.selected, in.hovered = noField, noField
	in.pdus[0] = newPDUView("请求", e.Request, decode.Request(e.Request), "")
	switch {
	case e.Err != nil:
		in.pdus[1] = newPDUView("响应", nil, nil, "无响应："+e.Err.Error())
	default:
		in.pdus[1] = newPDUView("响应", e.Response, decode.Response(e.Response, e.Request), "")
	}
}

func newPDUView(title string, data []byte, fields []decode.Field, note string) pduView {
	return pduView{
		title:  title,
		data:   data,
		fields: fields,
		note:   note,
		rows:   make([]widget.Clickable, len(fields)),
		bytes:  make([]widget.Clickable, len(data)),
		chars:  make([]widget.Clickable, len(data)),
	}
}

// fieldAt returns the innermost field of pdu covering byte i.
func (in *inspector) fieldAt(pdu, i int) fieldRef {
	ref := noField
	best := -1
	for k, f := range in.pdus[pdu].fields {
		if f.Start <= i && i < f.End && f.Depth > best {
			ref, best = fieldRef{pdu, k}, f.Depth
		}
	}
	return ref
}

func (in *inspector) layout(gtx C, th *Theme, e *telemetry.Event) D {
	if e == nil {
		return layout.Center.Layout(gtx, th.label("选择一条请求，查看解码字段与原始字节", textSize, colFaint).Layout)
	}
	in.load(e)
	in.update(gtx)
	return layout.Flex{}.Layout(gtx,
		layout.Flexed(0.5, func(gtx C) D { return in.fieldsPane(gtx, th) }),
		layout.Rigid(func(gtx C) D { return vline(gtx, colHair) }),
		layout.Flexed(0.5, func(gtx C) D { return in.hexPane(gtx, th) }),
	)
}

// update handles clicks and hover from the previous frame's widgets.
func (in *inspector) update(gtx C) {
	in.hovered = noField
	for p := range in.pdus {
		v := &in.pdus[p]
		for k := range v.rows {
			if v.rows[k].Clicked(gtx) {
				in.selected = fieldRef{p, k}
			}
			if v.rows[k].Hovered() {
				in.hovered = fieldRef{p, k}
			}
		}
		for i := range v.bytes {
			if v.bytes[i].Clicked(gtx) || v.chars[i].Clicked(gtx) {
				in.selected = in.fieldAt(p, i)
			}
			if v.bytes[i].Hovered() || v.chars[i].Hovered() {
				in.hovered = in.fieldAt(p, i)
			}
		}
	}
}

func paneTitle(th *Theme, txt string) layout.Widget {
	return func(gtx C) D {
		return layout.Inset{Left: 12, Top: 8, Bottom: 4}.Layout(gtx, th.bold(txt, smallSize, colMuted).Layout)
	}
}

func (in *inspector) fieldsPane(gtx C, th *Theme) D {
	type item struct {
		pdu, field int // field -1 = section heading
	}
	var items []item
	for p := range in.pdus {
		items = append(items, item{p, -1})
		for k := range in.pdus[p].fields {
			items = append(items, item{p, k})
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(paneTitle(th, "解码字段")),
		layout.Flexed(1, func(gtx C) D {
			return material.List(th.Theme, &in.fieldList).Layout(gtx, len(items), func(gtx C, i int) D {
				it := items[i]
				v := &in.pdus[it.pdu]
				if it.field < 0 {
					return in.sectionHeading(gtx, th, v)
				}
				return in.fieldRow(gtx, th, fieldRef{it.pdu, it.field})
			})
		}),
	)
}

func (in *inspector) sectionHeading(gtx C, th *Theme, v *pduView) D {
	return layout.Inset{Left: 12, Right: 12, Top: 6, Bottom: 2}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(th.bold(v.title, textSize, colInk).Layout),
			gap(8),
			layout.Rigid(func(gtx C) D {
				if v.note != "" {
					return th.label(v.note, smallSize, colErr).Layout(gtx)
				}
				return th.label(fmt.Sprintf("%d 字节", len(v.data)), smallSize, colFaint).Layout(gtx)
			}),
		)
	})
}

func (in *inspector) fieldRow(gtx C, th *Theme, ref fieldRef) D {
	v := &in.pdus[ref.pdu]
	f := v.fields[ref.field]
	btn := &v.rows[ref.field]
	bg := colCanvas
	switch {
	case in.selected == ref:
		bg = colAccentMd
	case in.hovered == ref:
		bg = colAccentBg
	}
	valueCol := colInk
	if f.Err {
		valueCol = colErr
	}
	return btn.Layout(gtx, func(gtx C) D {
		return background(gtx, bg, func(gtx C) D {
			return layout.Inset{Left: 12 + unit.Dp(f.Depth)*16, Right: 12}.Layout(gtx, func(gtx C) D {
				return row(gtx, 22,
					layout.Rigid(func(gtx C) D { return fixed(gtx, 96, th.label(f.Label, textSize, colMuted).Layout) }),
					layout.Flexed(1, th.label(f.Value, textSize, valueCol).Layout),
					layout.Rigid(func(gtx C) D {
						if f.Start == f.End {
							return D{}
						}
						return th.mono(byteRange(f), colFaint).Layout(gtx)
					}),
				)
			})
		})
	})
}

func byteRange(f decode.Field) string {
	if f.End-f.Start == 1 {
		return fmt.Sprintf("[%d]", f.Start)
	}
	return fmt.Sprintf("[%d..%d]", f.Start, f.End-1)
}

func (in *inspector) hexPane(gtx C, th *Theme) D {
	// One list item per hex row, with a title item before each PDU.
	type item struct {
		pdu, row int // row -1 = title
	}
	var items []item
	for p := range in.pdus {
		items = append(items, item{p, -1})
		for r := 0; r*bytesPerRow < len(in.pdus[p].data); r++ {
			items = append(items, item{p, r})
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(paneTitle(th, "原始字节（PDU）")),
		layout.Flexed(1, func(gtx C) D {
			return material.List(th.Theme, &in.hexList).Layout(gtx, len(items), func(gtx C, i int) D {
				it := items[i]
				v := &in.pdus[it.pdu]
				if it.row < 0 {
					return layout.Inset{Left: 12, Top: 6, Bottom: 2}.Layout(gtx, func(gtx C) D {
						txt := v.title
						if len(v.data) == 0 {
							txt += "：无数据"
						}
						return th.bold(txt, textSize, colInk).Layout(gtx)
					})
				}
				return in.hexRow(gtx, th, it.pdu, it.row)
			})
		}),
	)
}

// byteColors returns the text and background of byte i of pdu.
func (in *inspector) byteColors(pdu, i int) (fg, bg color.NRGBA) {
	covers := func(ref fieldRef) bool {
		if ref.pdu != pdu {
			return false
		}
		f := in.pdus[pdu].fields[ref.field]
		return f.Start <= i && i < f.End
	}
	switch {
	case in.selected != noField && covers(in.selected):
		return colInk, colAccentMd
	case in.hovered != noField && covers(in.hovered):
		return colInk, colAccentBg
	}
	return colBody, colCanvas
}

func (in *inspector) hexRow(gtx C, th *Theme, pdu, r int) D {
	v := &in.pdus[pdu]
	lo := r * bytesPerRow
	hi := min(lo+bytesPerRow, len(v.data))
	children := []layout.FlexChild{
		layout.Rigid(func(gtx C) D { return fixed(gtx, 44, th.mono(fmt.Sprintf("%04X", lo), colFaint).Layout) }),
	}
	for i := lo; i < lo+bytesPerRow; i++ {
		if i == lo+bytesPerRow/2 {
			children = append(children, gap(6))
		}
		if i >= hi {
			children = append(children, layout.Rigid(layout.Spacer{Width: 22}.Layout))
			continue
		}
		children = append(children, layout.Rigid(func(gtx C) D {
			fg, bg := in.byteColors(pdu, i)
			return v.bytes[i].Layout(gtx, func(gtx C) D {
				return fixed(gtx, 22, func(gtx C) D {
					return background(gtx, bg, func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Center.Layout(gtx, th.mono(fmt.Sprintf("%02X", v.data[i]), fg).Layout)
					})
				})
			})
		}))
	}
	children = append(children, gap(12))
	for i := lo; i < hi; i++ {
		children = append(children, layout.Rigid(func(gtx C) D {
			fg, bg := in.byteColors(pdu, i)
			return v.chars[i].Layout(gtx, func(gtx C) D {
				return fixed(gtx, 9, func(gtx C) D {
					return background(gtx, bg, func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Center.Layout(gtx, th.mono(printable(v.data[i]), fg).Layout)
					})
				})
			})
		}))
	}
	return layout.Inset{Left: 12}.Layout(gtx, func(gtx C) D {
		return row(gtx, 20, children...)
	})
}

func printable(b byte) string {
	if b >= 0x20 && b < 0x7F {
		return string(rune(b))
	}
	return "."
}
