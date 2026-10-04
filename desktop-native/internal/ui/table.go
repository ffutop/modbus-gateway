// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package ui

import (
	"fmt"
	"time"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/decode"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/store"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

const rowHeight unit.Dp = 24

type column struct {
	title string
	width unit.Dp // 0 = take the remaining width
	mono  bool
	text  func(e *telemetry.Event) string
}

var columns = []column{
	{"序号", 64, true, func(e *telemetry.Event) string { return fmt.Sprint(e.Seq) }},
	{"时间", 112, true, func(e *telemetry.Event) string { return e.Time.Format("15:04:05.000") }},
	{"网关", 110, false, func(e *telemetry.Event) string { return e.Gateway }},
	{"来源", 150, true, func(e *telemetry.Event) string { return e.Source }},
	{"下游", 110, false, func(e *telemetry.Event) string { return e.Downstream }},
	{"从站", 44, true, func(e *telemetry.Event) string { return fmt.Sprint(e.SlaveID) }},
	{"功能码", 120, false, func(e *telemetry.Event) string {
		return fmt.Sprintf("%02X %s", e.FunctionCode, decode.FunctionName(e.FunctionCode))
	}},
	{"地址", 60, true, func(e *telemetry.Event) string { return fmt.Sprint(e.Address) }},
	{"数量", 48, true, func(e *telemetry.Event) string { return fmt.Sprint(e.Quantity) }},
	{"耗时", 76, true, func(e *telemetry.Event) string {
		return fmt.Sprintf("%.2f ms", float64(e.Duration)/float64(time.Millisecond))
	}},
	{"结果", 0, false, func(e *telemetry.Event) string {
		if e.Err != nil {
			return e.Err.Error()
		}
		return "正常"
	}},
}

// requestTable is the live, virtualized request list. Rows are keyed by
// Seq, so a click stays on its request while new ones scroll the list.
type requestTable struct {
	list   widget.List
	rows   map[uint64]*widget.Clickable
	seen   map[uint64]bool
	sel    telemetry.Event // a copy, so it outlives the store's ring
	hasSel bool
}

func (t *requestTable) init() {
	t.list.Axis = layout.Vertical
	t.list.ScrollToEnd = true
	t.rows = map[uint64]*widget.Clickable{}
	t.seen = map[uint64]bool{}
}

func (t *requestTable) selected() *telemetry.Event {
	if !t.hasSel {
		return nil
	}
	return &t.sel
}

func (t *requestTable) layout(gtx C, th *Theme, snap store.Snapshot) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return background(gtx, colSoft, func(gtx C) D {
				return t.cells(gtx, func(c column) material.LabelStyle {
					return th.label(c.title, smallSize, colMuted)
				})
			})
		}),
		layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }),
		layout.Flexed(1, func(gtx C) D {
			if snap.Len() == 0 {
				return layout.Center.Layout(gtx, th.label("暂无请求。等待上游主站发起请求…", textSize, colFaint).Layout)
			}
			clear(t.seen)
			d := material.List(th.Theme, &t.list).Layout(gtx, snap.Len(), func(gtx C, i int) D {
				return t.row(gtx, th, snap.At(i))
			})
			for seq := range t.rows {
				if !t.seen[seq] {
					delete(t.rows, seq)
				}
			}
			return d
		}),
	)
}

func (t *requestTable) row(gtx C, th *Theme, e *telemetry.Event) D {
	t.seen[e.Seq] = true
	btn := t.rows[e.Seq]
	if btn == nil {
		btn = new(widget.Clickable)
		t.rows[e.Seq] = btn
	}
	if btn.Clicked(gtx) {
		t.sel, t.hasSel = *e, true
	}
	selected := t.hasSel && t.sel.Seq == e.Seq
	bg := colCanvas
	switch {
	case selected:
		bg = colAccentBg
	case btn.Hovered():
		bg = colSoft
	}
	return btn.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return background(gtx, bg, func(gtx C) D {
					return t.cells(gtx, func(c column) material.LabelStyle {
						col := colBody
						if c.title == "结果" {
							col = colOk
							if e.Err != nil {
								col = colErr
							}
						}
						if c.mono {
							return th.mono(c.text(e), col)
						}
						return th.label(c.text(e), textSize, col)
					})
				})
			}),
			layout.Rigid(func(gtx C) D { return hline(gtx, colHairSoft) }),
		)
	})
}

// cells lays out one table row, one label per column.
func (t *requestTable) cells(gtx C, label func(column) material.LabelStyle) D {
	children := make([]layout.FlexChild, 0, len(columns))
	for _, c := range columns {
		cell := func(gtx C) D {
			return layout.Inset{Left: 8, Right: 4}.Layout(gtx, label(c).Layout)
		}
		if c.width == 0 {
			children = append(children, layout.Flexed(1, cell))
		} else {
			children = append(children, layout.Rigid(func(gtx C) D { return fixed(gtx, c.width, cell) }))
		}
	}
	return row(gtx, rowHeight, children...)
}
