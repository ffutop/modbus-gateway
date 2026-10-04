// Package workspace implements the approved desktop workspace over live gateway data.
package workspace

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/decode"
)

// detailFunc draws one expanded exchange.
type detailFunc func(gtx C, x *Exchange) D

type linkedView struct {
	th    *Theme
	world *World

	rows                            clicks[uint64]
	sel                             uint64
	selX                            Exchange
	tables                          [4]widget.Clickable
	table                           table
	regs                            [tableSize]widget.Clickable
	reg                             int
	clearRg                         widget.Clickable
	masters                         clicks[string]
	master                          string
	slave                           int
	slaves                          clicks[int]
	packetButtons                   clicks[uint64]
	copyButtons                     clicks[uint64]
	packets                         map[uint64]bool
	paused                          bool
	pause, errorsOnly, clearFilters widget.Clickable
	onlyErrors                      bool

	clearMaster bool
	traffic     widget.List
	regList     widget.List
	slaveList   widget.List
}

func newLinkedView(th *Theme, w *World) *linkedView {
	v := &linkedView{th: th, world: w, rows: clicks[uint64]{}, masters: clicks[string]{}, reg: -1, slave: -1, slaves: clicks[int]{}, packetButtons: clicks[uint64]{}, copyButtons: clicks[uint64]{}, packets: map[uint64]bool{}}
	v.traffic.Axis, v.traffic.ScrollToEnd = layout.Vertical, true
	v.regList.Axis = layout.Vertical
	v.slaveList.Axis = layout.Horizontal
	return v
}

// reset clears selections that belong to the previous link.
func (v *linkedView) reset() { v.sel, v.reg, v.master, v.slave = 0, -1, "", -1 }

func touches(x *Exchange, t table, addr int) bool {
	et, start, count, ok := affectedRange(x)
	return ok && et == t && addr >= start && addr < start+count
}

// Layout draws traffic for link l beside the registers of its downstream.
// showMasters adds master chips, for selectors that pick only a downstream.
func (v *linkedView) Layout(gtx C, l Link, showMasters bool, detail detailFunc) D {
	for i := range v.tables {
		if v.tables[i].Clicked(gtx) {
			v.table, v.reg = table(i), -1
		}
	}
	for i := range v.regs {
		if v.regs[i].Clicked(gtx) {
			if v.reg == i {
				v.reg = -1
			} else {
				v.reg = i
			}
		}
	}
	if v.clearRg.Clicked(gtx) {
		v.reg = -1
	}
	for m, b := range v.masters {
		if b.Clicked(gtx) {
			v.master = m
		}
	}
	if showMasters && v.master != "" {
		l.Master = v.master
	}

	for id, btn := range v.slaves {
		if btn.Clicked(gtx) {
			v.slave = id
			v.sel = 0
			v.reg = -1
		}
	}
	if v.pause.Clicked(gtx) {
		if v.paused || v.sel != 0 {
			v.paused = false
			v.sel = 0
		} else {
			v.paused = true
		}
	}
	if v.errorsOnly.Clicked(gtx) {
		v.onlyErrors = !v.onlyErrors
	}
	if v.clearFilters.Clicked(gtx) {
		v.reg = -1
		v.master = ""
		v.onlyErrors = false
		v.clearMaster = true
	}
	if l.Ds != nil && !l.Ds.InProcess() {
		if v.slave < 0 {
			v.slave = int(l.Ds.IDs()[0])
		}
		l.SlaveFilter = v.slave + 1
	}
	registerLink := l
	registerLink.Master = ""
	registerEvents := v.world.Exchanges(registerLink.Match)
	// Keep an expanded request in view instead of following new traffic.
	v.traffic.ScrollToEnd = v.sel == 0 && !v.paused

	all := v.world.Exchanges(l.Match)
	var reads, writes [tableSize]int
	shown := all[:0:0]
	for i := range all {
		x := &all[i]
		if t, start, count, ok := affectedRange(x); ok && t == v.table && x.Err == nil {
			for a := start; a < start+count && a < tableSize; a++ {
				if isWrite(x.FunctionCode) {
					writes[a]++
				} else {
					reads[a]++
				}
			}
		}
		if (!v.onlyErrors || failed(&x.Event)) && (v.reg < 0 || touches(x, v.table, v.reg)) {
			shown = append(shown, *x)
		}
	}
	return layout.Flex{}.Layout(gtx,
		layout.Flexed(0.58, func(gtx C) D { return v.trafficPane(gtx, l, showMasters, shown, len(all), detail) }),
		layout.Rigid(func(gtx C) D { return vline(gtx, colHair) }),
		layout.Flexed(0.42, func(gtx C) D { return v.registerPane(gtx, registerLink, registerEvents, &reads, &writes) }),
	)
}

func (v *linkedView) trafficRow(gtx C, l Link, x *Exchange, detail detailFunc) D {
	th := v.th
	btn := v.rows.get(x.Seq)
	if btn.Clicked(gtx) {
		if v.sel == x.Seq {
			v.sel = 0
		} else {
			v.selectRequest(*x)
		}
	}
	open := v.sel == x.Seq
	bg := colCanvas
	rowBody, rowMuted := colBody, colMuted
	switch {
	case open:
		bg = colHover
		rowBody, rowMuted = colInk, colBody
	case btn.Hovered():
		bg = colSoft
	}
	res, resCol := "正常", rowMuted
	if failed(&x.Event) {
		res, resCol = "异常", colErr
	}
	path := ""
	if l.Master == "" && v.master == "" {
		path = shortMaster(x.Source)
	}
	if l.Ds == nil {
		if path != "" {
			path += " → "
		}
		if x.Ds != nil {
			path += x.Ds.Name
		} else {
			path += fmt.Sprintf("从站 %d（无路由）", x.SlaveID)
		}
	}
	head := func(gtx C) D {
		return btn.Layout(gtx, func(gtx C) D {
			return background(gtx, bg, func(gtx C) D {
				return layout.Inset{Left: 10, Right: 14}.Layout(gtx, func(gtx C) D {
					cols := requestColumns(gtx, l, v.master)
					children := []layout.FlexChild{
						layout.Rigid(func(gtx C) D { return fixed(gtx, 16, func(gtx C) D { return disclosure(gtx, open, rowMuted) }) }),
						layout.Rigid(func(gtx C) D { return fixed(gtx, cols[0], th.mono(x.Time.Format("15:04:05.000"), rowMuted).Layout) }),
					}
					if path != "" {
						pw := cols[1]
						children = append(children, layout.Rigid(func(gtx C) D { return fixed(gtx, pw, th.label(path, smallSize, rowMuted).Layout) }))
					}
					children = append(children,
						layout.Rigid(func(gtx C) D {
							return fixed(gtx, cols[2], func(gtx C) D {
								return layout.Inset{Right: 8}.Layout(gtx, th.label(fcText(&x.Event), textSize, rowBody).Layout)
							})
						}),
						layout.Rigid(func(gtx C) D {
							return fixed(gtx, cols[3], th.mono(fmt.Sprintf("@%d ×%d", x.Address, x.Quantity), rowBody).Layout)
						}),
						layout.Rigid(func(gtx C) D { return fixed(gtx, cols[4], th.mono(durText(x.Duration), rowBody).Layout) }),
						layout.Flexed(1, func(gtx C) D {
							if open && failed(&x.Event) {
								return th.badge(gtx, res, colErr, colErrBg)
							}
							return th.label(res, textSize, resCol).Layout(gtx)
						}),
					)
					return row(gtx, 24, children...)
				})
			})
		})
	}
	if !open {
		return head(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(head),
		layout.Rigid(func(gtx C) D {
			return background(gtx, colSoft, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Left: 26, Right: 14, Top: 6, Bottom: 10}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx C) D { return detail(gtx, x) }), layout.Rigid(func(gtx C) D { return v.packetPane(gtx, x) }))
				})
			})
		}),
	)
}

func shortMaster(m string) string {
	if i := strings.IndexByte(m, ' '); i > 0 {
		return m[:i]
	}
	return m
}

// observed rebuilds a real device's registers from the read responses seen
// on the link.
func observed(xs []Exchange, t table) (vals [tableSize]uint16, seen [tableSize]time.Time) {
	for i := range xs {
		x := &xs[i]
		if isWrite(x.FunctionCode) {
			continue
		}
		xt, a, _, ok := affectedRange(x)
		if !ok || xt != t {
			continue
		}
		for k, v := range requestValues(x) {
			if a+k < tableSize {
				vals[a+k], seen[a+k] = v, x.Time
			}
		}
	}
	return
}

func (v *linkedView) accessBar(gtx C, reads, writes, maxAccess int) D {
	th := v.th
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx C) D {
			w := gtx.Constraints.Max.X
			h := gtx.Dp(5)
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					gtx.Constraints.Min = image.Pt(w*reads/maxAccess, h)
					return fill(gtx, colHair)
				}),
				layout.Rigid(func(gtx C) D {
					gtx.Constraints.Min = image.Pt(w*writes/maxAccess, h)
					return fill(gtx, colInk)
				}),
			)
		}),
		gap(8),
		layout.Rigid(func(gtx C) D {
			return fixed(gtx, 56, th.label(fmt.Sprintf("读%d 写%d", reads, writes), smallSize, colMuted).Layout)
		}),
	)
}

// ---- detail renderer 1: sequence diagram ----

// sequence draws master | gateway | device lanes with the four messages of
// one exchange. An in-process model gets a dashed lane and operations
// instead of frames.
func (th *Theme) sequence(gtx C, x *Exchange) D {
	type step struct {
		from, to int // lane index
		title    string
		at       time.Duration
		bytes    []byte
		note     string
		col      color.NRGBA
		dashed   bool
	}
	inProc := x.Ds != nil && x.Ds.InProcess()
	lane3 := "（无路由）"
	if x.Ds != nil {
		lane3 = fmt.Sprintf("%s · ID %d", x.Ds.Name, x.SlaveID)
		if inProc {
			lane3 = "模型 " + x.Ds.Sim
		}
	}
	lanes := []string{x.Source, x.Gw.Name + " 网关", lane3}
	protos := []string{x.Gw.UpProto(), "", ""}
	if x.Ds != nil {
		protos[2] = x.Ds.Proto()
		if inProc {
			protos[2] = x.Ds.Type + " · 进程内"
		}
	}
	opText := fmt.Sprintf("%s @%d ×%d", decode.FunctionName(x.FunctionCode), x.Address, x.Quantity)
	if x.Ds != nil && x.Ds.Type == "injector" {
		if t, a, n, ok := affectedRange(x); ok {
			opText = fmt.Sprintf("%s @%d → %s @%d ×%d", strings.Fields(tableNames[holding])[0], x.Address, strings.Fields(tableNames[t])[0], a, n)
		}
	}
	steps := []step{{0, 1, "① 网关收到请求 PDU", 0, x.Request, "上游 ADU 未采集", colBody, false}}
	if x.Ds == nil {
		steps = append(steps, step{1, 1, "", 0, nil, "无匹配路由", colErr, false})
	} else {
		kind := "② 转发调用"
		if inProc {
			kind = "② 调用模型"
		}
		steps = append(steps, step{1, 2, kind, 0, nil, "单段时间未采集", colBody, inProc})
		note, col := "返回 PDU", colBody
		if failed(&x.Event) {
			note, col = "异常返回", colErr
		}
		steps = append(steps, step{2, 1, "③ " + note, 0, x.Response, "单段时间未采集", col, inProc})
	}
	steps = append(steps, step{1, 0, "④ 网关处理完成", x.Duration, nil, "上游回复 ADU 未采集", colBody, false})

	laneX := func(width, i int) int { return width * (2*i + 1) / 6 }
	children := []layout.FlexChild{
		// lane headers
		layout.Rigid(func(gtx C) D {
			w := gtx.Constraints.Max.X
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx C) D { return D{Size: image.Pt(w, gtx.Dp(48))} }),
				layout.Stacked(func(gtx C) D {
					for i, name := range lanes {
						cw := w / 3
						off := op.Offset(image.Pt(i*cw, 0)).Push(gtx.Ops)
						g := gtx
						g.Constraints = layout.Exact(image.Pt(cw, gtx.Dp(48)))
						layout.Center.Layout(g, func(gtx C) D {
							bg, fg := colCanvas, colInk
							if i == 2 && inProc {
								bg, fg = colSoft, colBody
							}
							return outlined(gtx, colHair, bg, radiusSm, func(gtx C) D {
								return layout.Inset{Left: 8, Right: 8, Top: 3, Bottom: 3}.Layout(gtx, func(gtx C) D {
									return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(th.bold(name, smallSize, fg).Layout),
										layout.Rigid(th.label(protos[i], microSize, colMuted).Layout))
								})
							})
						})
						off.Pop()
					}
					return D{Size: image.Pt(w, gtx.Dp(48))}
				}),
			)
		}),
	}
	for _, s := range steps {
		children = append(children, layout.Rigid(func(gtx C) D {
			w := gtx.Constraints.Max.X
			h := gtx.Dp(30)
			x1, x2 := laneX(w, s.from), laneX(w, s.to)
			y := gtx.Dp(25)
			// lanes
			for i := range lanes {
				lx := laneX(w, i)
				r := clip.Rect{Min: image.Pt(lx, 0), Max: image.Pt(lx+1, h)}.Push(gtx.Ops)
				lc := colHair
				if i == 2 && inProc && x.Ds.Type == "injector" {
					lc = colAccentMd
				}
				paint.Fill(gtx.Ops, lc)
				r.Pop()
			}
			if x1 != x2 {
				stroke := s.col
				if inProc && x.Ds.Type == "injector" && s.dashed {
					stroke = colAccent
				}
				drawArrow(gtx, x1, x2, y, stroke, s.dashed)
			}
			// label above the arrow, between the lanes
			lx := min(x1, x2) + gtx.Dp(8)
			if x1 == x2 {
				lx = x1 + gtx.Dp(10)
			}
			off := op.Offset(image.Pt(lx, 0)).Push(gtx.Ops)
			g := gtx
			g.Constraints = layout.Constraints{Max: image.Pt(w-lx, h)}
			layout.Flex{Alignment: layout.Baseline}.Layout(g,
				layout.Rigid(th.label(s.title, smallSize, s.col).Layout),
				gap(6),
				layout.Rigid(func(gtx C) D {
					if s.at == 0 {
						return D{}
					}
					return th.mono("总耗时 "+durText(s.at), colMuted).Layout(gtx)
				}),
				gap(8),
				layout.Rigid(th.label(s.note, smallSize, s.col).Layout),
			)
			off.Pop()
			return D{Size: image.Pt(w, h)}
		}))

	}
	if x.Err != nil {
		children = append(children, layout.Rigid(func(gtx C) D {
			txt := x.Err.Error()
			if x.Ds == nil {
				txt = fmt.Sprintf("Slave ID %d 没有匹配的路由。上游实际回复未采集。", x.SlaveID)
			}
			l := th.label(txt, smallSize, colErr)
			l.MaxLines = 0
			return l.Layout(gtx)
		}))
	}
	if inProc {
		children = append(children, layout.Rigid(func(gtx C) D { l := th.label(opText, smallSize, colBody); l.MaxLines = 0; return l.Layout(gtx) }))
	}
	children = append(children, vgap(6), layout.Rigid(func(gtx C) D { return th.pduSummary(gtx, x) }))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// pduSummary is a one-line decode of the request and response PDUs.
func (th *Theme) pduSummary(gtx C, x *Exchange) D {
	line := func(title string, fields []decode.Field) layout.FlexChild {
		var parts []string
		for _, f := range fields {
			if f.Depth == 0 && f.Value != "" && f.Label != "功能码" {
				parts = append(parts, f.Label+" "+f.Value)
			}
		}
		return layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return fixed(gtx, 64, th.label(title, smallSize, colMuted).Layout) }),
				layout.Flexed(1, th.label(strings.Join(parts, " · "), smallSize, colBody).Layout))
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		line("请求 PDU", decode.Request(x.Request)),
		line("响应 PDU", decode.Response(x.Response, x.Request)),
	)
}

func drawArrow(gtx C, x1, x2, y int, col color.NRGBA, dashed bool) {
	w := float32(gtx.Dp(1.5))
	seg := func(a, b int) {
		var p clip.Path
		p.Begin(gtx.Ops)
		p.MoveTo(f32.Pt(float32(a), float32(y)))
		p.LineTo(f32.Pt(float32(b), float32(y)))
		paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: w}.Op())
	}
	dir := 1
	if x2 < x1 {
		dir = -1
	}
	head := gtx.Dp(7)
	end := x2 - dir*head
	if dashed {
		dash, gapPx := gtx.Dp(5), gtx.Dp(4)
		for a := x1; (a-end)*dir < 0; a += dir * (dash + gapPx) {
			b := a + dir*dash
			if (b-end)*dir > 0 {
				b = end
			}
			seg(a, b)
		}
	} else {
		seg(x1, end)
	}
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(float32(x2), float32(y)))
	p.LineTo(f32.Pt(float32(x2-dir*head), float32(y-head/2)))
	p.LineTo(f32.Pt(float32(x2-dir*head), float32(y+head/2)))
	p.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
}

// ---- detail renderer 2: side-by-side frames ----

type frameField struct {
	label, value string
	raw          []byte
	changed      bool // rewritten by the gateway
}

// frameFields splits an ADU into its envelope and PDU fields.
func frameFields(kind string, adu []byte, pdu, req []byte, isReq bool) []frameField {
	if len(adu) == 0 {
		return nil
	}
	var env, tail []frameField
	switch kind {
	case "tcp":
		env = []frameField{
			{label: "事务号", value: fmt.Sprint(binary.BigEndian.Uint16(adu[0:])), raw: adu[0:2]},
			{label: "协议号", value: "0", raw: adu[2:4]},
			{label: "长度", value: fmt.Sprint(binary.BigEndian.Uint16(adu[4:])), raw: adu[4:6]},
			{label: "单元号", value: fmt.Sprint(adu[6]), raw: adu[6:7]},
		}
	default:
		env = []frameField{{label: "从站地址", value: fmt.Sprint(adu[0]), raw: adu[0:1]}}
		tail = []frameField{{label: "CRC", value: fmt.Sprintf("0x%02X%02X", adu[len(adu)-1], adu[len(adu)-2]), raw: adu[len(adu)-2:]}}
	}
	var fs []decode.Field
	if isReq {
		fs = decode.Request(pdu)
	} else {
		fs = decode.Response(pdu, req)
	}
	for _, f := range fs {
		var raw []byte
		if f.End > f.Start {
			raw = pdu[f.Start:f.End]
		}
		label := f.Label
		if f.Depth > 0 {
			label = "  " + label
		}
		env = append(env, frameField{label: label, value: f.Value, raw: raw})
	}
	return append(env, tail...)
}

// framesCompare shows the upstream and downstream frames in two columns,
// marking what the gateway rewrote.
func (th *Theme) framesCompare(gtx C, x *Exchange) D {
	upKind := x.Gw.UpType
	downKind := ""
	if x.Ds != nil {
		downKind = x.Ds.Type
	}
	col := func(title, proto string, req, resp []frameField, note string, accent bool) layout.Widget {
		return func(gtx C) D {
			children := []layout.FlexChild{layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
					layout.Rigid(th.bold(title, textSize, colInk).Layout), gap(6),
					layout.Rigid(func(gtx C) D {
						if accent {
							return th.badge(gtx, proto, colBody, colCard)
						}
						return th.badge(gtx, proto, colMuted, colCard)
					}))
			}), vgap(4)}
			if note != "" {
				children = append(children, layout.Rigid(th.label(note, smallSize, colBody).Layout))
			}
			section := func(name string, fs []frameField) {
				if len(fs) == 0 {
					return
				}
				children = append(children, layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: 4, Bottom: 2}.Layout(gtx, th.bold(name, smallSize, colMuted).Layout)
				}))
				for _, f := range fs {
					children = append(children, layout.Rigid(func(gtx C) D {
						bg := colSoft
						if f.changed {
							bg = colWarnBg
						}
						return background(gtx, bg, func(gtx C) D {
							return row(gtx, 19,
								layout.Rigid(func(gtx C) D { return fixed(gtx, 70, th.label(f.label, smallSize, colMuted).Layout) }),
								layout.Rigid(func(gtx C) D { return fixed(gtx, 64, th.mono(fmt.Sprintf("% X", f.raw), colMuted).Layout) }),
								layout.Flexed(1, th.label(f.value, smallSize, colBody).Layout),
							)
						})
					}))
				}
			}
			section("请求帧", req)
			section("响应帧", resp)
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}
	}
	upReq := frameFields(upKind, x.UpReq, x.Request, nil, true)
	upResp := frameFields(upKind, x.UpResp, x.Response, x.Request, false)
	var downReq, downResp []frameField
	note := ""
	switch {
	case x.Ds == nil:
		note = "无路由：没有下游段"
	case x.Ds.InProcess():
		note = fmt.Sprintf("进程内调用模型 %s：%s @%d ×%d，无线路帧", x.Ds.Sim, decode.FunctionName(x.FunctionCode), x.Address, x.Quantity)
	default:
		downReq = frameFields(downKind, x.DownReq, x.Request, nil, true)
		downResp = frameFields(downKind, x.DownResp, x.Response, x.Request, false)
		if x.DownErr != "" {
			note = x.DownErr
		}
		markRewrites(upReq, downReq, upKind != downKind)
		markRewrites(upResp, downResp, upKind != downKind)
	}
	downProto := "—"
	if x.Ds != nil {
		downProto = x.Ds.Proto()
	}
	return layout.Flex{}.Layout(gtx,
		layout.Flexed(0.5, col("上游段 · "+shortMaster(x.Source)+" ↔ 网关", x.Gw.UpProto(), upReq, upResp, "", false)),
		gap(14),
		layout.Flexed(0.5, col("下游段 · 网关 ↔ "+dsName(x), downProto, downReq, downResp, note, x.Ds != nil && x.Ds.InProcess())),
	)
}

func dsName(x *Exchange) string {
	if x.Ds == nil {
		return fmt.Sprintf("从站 %d", x.SlaveID)
	}
	return x.Ds.Name
}

// markRewrites flags envelope fields that differ between the two legs; when
// the framing changes, every envelope field counts as rewritten.
func markRewrites(up, down []frameField, reframed bool) {
	byLabel := map[string][]byte{}
	for _, f := range up {
		byLabel[f.label] = f.raw
	}
	for i := range down {
		raw, ok := byLabel[down[i].label]
		switch {
		case !ok:
			down[i].changed = reframed
		case string(raw) != string(down[i].raw):
			down[i].changed = true
		}
	}
}

func (v *linkedView) selectRequest(x Exchange) {
	v.sel, v.selX = x.Seq, x
	if t, a, _, ok := affectedRange(&x); ok {
		v.table = t
		v.regList.Position.First = min(max(a-2, 0), tableSize-1)
		v.regList.Position.Offset = 0
	}
}
