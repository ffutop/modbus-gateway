// Package workspace implements the approved desktop workspace over live gateway data.
package workspace

import (
	"fmt"
	"image"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func requestColumns(gtx C, l Link, master string) [5]unit.Dp {
	path := unit.Dp(0)
	if l.Master == "" && master == "" || l.Ds == nil {
		path = 66
	}
	width := float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
	if width < 540 {
		if path > 0 {
			path = 54
		}
		return [5]unit.Dp{94, path, 98, 64, 60}
	}
	return [5]unit.Dp{94, path, 112, 76, 66}
}

func (v *linkedView) trafficPane(gtx C, l Link, showMasters bool, xs []Exchange, total int, detail detailFunc) D {
	th := v.th
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx C) D {
				return row(gtx, 34,
					layout.Rigid(th.bold("请求", textSize, colInk).Layout), gap(8),
					layout.Rigid(th.label(fmt.Sprintf("%d / %d", len(xs), total), smallSize, colMuted).Layout),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx C) D { return th.chip(gtx, &v.errorsOnly, "仅异常", v.onlyErrors) }), gap(4),
					layout.Rigid(func(gtx C) D {
						txt := "暂停跟随"
						if v.paused || v.sel != 0 {
							txt = "恢复实时"
						}
						return th.button(gtx, &v.pause, txt, false)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx C) D {
			if v.reg < 0 && l.Master == "" && v.master == "" && !v.onlyErrors {
				return D{}
			}
			return layout.Inset{Left: 14, Right: 14, Bottom: 4}.Layout(gtx, func(gtx C) D {
				scope := l.Master
				if scope == "" {
					scope = v.master
				}
				if v.reg >= 0 {
					scope += fmt.Sprintf(" · %s @%d", strings.Fields(tableNames[v.table])[0], v.reg)
				}
				if v.onlyErrors {
					scope += " · 仅异常"
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, th.label(strings.Trim(scope, " ·"), smallSize, colMuted).Layout), gap(4), layout.Rigid(func(gtx C) D { return th.button(gtx, &v.clearFilters, "清除筛选", false) }))
			})
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 10, Right: 14}.Layout(gtx, func(gtx C) D {
				cols := requestColumns(gtx, l, v.master)
				children := []layout.FlexChild{gap(16)}
				labels := []string{"时间", "来源", "功能", "地址·数量", "耗时"}
				for i, w := range cols {
					if w == 0 {
						continue
					}
					label, width := labels[i], w
					children = append(children, layout.Rigid(func(gtx C) D { return fixed(gtx, width, th.label(label, smallSize, colMuted).Layout) }))
				}
				children = append(children, layout.Flexed(1, th.label("状态", smallSize, colMuted).Layout))
				return row(gtx, 24, children...)
			})
		}),
		layout.Rigid(func(gtx C) D { return hline(gtx, colHairSoft) }),
		layout.Flexed(1, func(gtx C) D {
			if len(xs) == 0 {
				txt := "这条链路上还没有请求"
				if total > 0 {
					txt = "没有符合当前筛选的请求"
				}
				return layout.Center.Layout(gtx, th.label(txt, textSize, colMuted).Layout)
			}
			return material.List(th.Theme, &v.traffic).Layout(gtx, len(xs), func(gtx C, i int) D {
				// Keep a partially scrolled request header from showing clipped glyphs
				// against the table heading; retain its geometry and any expanded detail.
				if i == v.traffic.Position.First && v.traffic.Position.Offset > 0 && v.traffic.Position.Offset < gtx.Dp(24) {
					defer clip.Rect{Min: image.Pt(0, gtx.Dp(24)), Max: image.Pt(gtx.Constraints.Max.X, 1<<20)}.Push(gtx.Ops).Pop()
				}
				return v.trafficRow(gtx, l, &xs[i], detail)
			})
		}),
	)
}

func (v *linkedView) registerPane(gtx C, l Link, xs []Exchange, reads, writes *[tableSize]int) D {
	th := v.th
	sim := l.Sim
	if l.Ds != nil {
		sim = l.Ds.Sim
	}
	if l.Ds == nil && sim == "" {
		return layout.Center.Layout(gtx, th.label("选择一个下游，查看它的寄存器", textSize, colMuted).Layout)
	}
	model := sim != ""
	vals, changed := v.world.Observed(l, v.table)
	if model {
		if v.world.models[sim] == nil {
			return layout.Center.Layout(gtx, th.label("模型未启动；请检查配置与启动日志", textSize, colErr).Layout)
		}
		vals, changed = v.world.Snapshot(sim, v.table)
	}
	var historic [tableSize]string
	lo, hi := -1, -1
	if v.sel != 0 {
		t, a, n, ok := affectedRange(&v.selX)
		if ok && t == v.table {
			lo, hi = a, a+n
			for i, value := range requestValues(&v.selX) {
				if a+i < tableSize {
					historic[a+i] = fmt.Sprint(value)
				}
			}
		}
	}
	title := "模型 " + sim
	provenance := "当前模型值 · 地址 0–119 · 多个入口共享四张表"
	if !model {
		title = fmt.Sprintf("%s / Slave ID %d", l.Ds.Name, v.slave)
		provenance = l.Gw.Name + " · 地址 0–119 · 最后读取值（全部主站）"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14, Right: 14, Top: 7}.Layout(gtx, th.bold(title, textSize, colInk).Layout)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14, Right: 14, Top: 3, Bottom: 3}.Layout(gtx, th.label(provenance, smallSize, colMuted).Layout)
		}),
		layout.Rigid(func(gtx C) D {
			if model || len(l.Ds.IDs()) < 2 {
				return D{}
			}
			return layout.Inset{Left: 14, Right: 14, Bottom: 4}.Layout(gtx, func(gtx C) D {
				return fixedH(gtx, 32, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(th.label("Slave ID", smallSize, colMuted).Layout), gap(8),
						layout.Flexed(1, func(gtx C) D {
							ids := l.Ds.IDs()
							return material.List(th.Theme, &v.slaveList).Layout(gtx, len(ids), func(gtx C, i int) D {
								id := int(ids[i])
								return layout.Inset{Right: 4}.Layout(gtx, func(gtx C) D {
									return fixedH(gtx, 32, func(gtx C) D { return th.chip(gtx, v.slaves.get(id), fmt.Sprint(id), v.slave == id) })
								})
							})
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14, Right: 10}.Layout(gtx, func(gtx C) D {
				children := []layout.FlexChild{}
				for i := range v.tables {
					i := i
					children = append(children, layout.Rigid(func(gtx C) D {
						return th.chip(gtx, &v.tables[i], strings.Fields(tableNames[i])[0], v.table == table(i))
					}), gap(3))
				}
				return row(gtx, 32, children...)
			})
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14, Right: 14, Bottom: 4}.Layout(gtx, func(gtx C) D {
				txt := "访问次数：当前保留请求"
				if v.sel != 0 {
					kind := "读值"
					if isWrite(v.selX.FunctionCode) {
						kind = "写入回显"
					}
					txt = "本请求：" + kind + " · 蓝底：关联范围"
				}
				return th.label(txt, smallSize, colMuted).Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx C) D {
				label := "当前值"
				if !model {
					label = "最后读值"
				}
				return registerRow(gtx, th, "地址", label, "HEX", "本请求", "读/写", "更新", true)
			})
		}),
		layout.Rigid(func(gtx C) D { return hline(gtx, colHairSoft) }),
		layout.Flexed(1, func(gtx C) D {
			return material.List(th.Theme, &v.regList).Layout(gtx, tableSize, func(gtx C, i int) D {
				bg := colCanvas
				if v.reg == i {
					bg = colAccentMd
				} else if i >= lo && i < hi {
					bg = colAccentBg
				} else if v.regs[i].Hovered() {
					bg = colSoft
				}
				value, hex := fmt.Sprint(vals[i]), fmt.Sprintf("0x%04X", vals[i])
				age := "—"
				if !changed[i].IsZero() {
					age = fmt.Sprintf("%.0fs", gtx.Now.Sub(changed[i]).Seconds())
				}
				if !model && changed[i].IsZero() {
					value, hex = "—", "未观测"
				}
				return v.regs[i].Layout(gtx, func(gtx C) D {
					return background(gtx, bg, func(gtx C) D {
						return layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx C) D {
							return registerRow(gtx, th, fmt.Sprint(i), value, hex, historic[i], fmt.Sprintf("%d/%d", reads[i], writes[i]), age, false)
						})
					})
				})
			})
		}),
	)
}

func registerRow(gtx C, th *Theme, address, value, hex, historic, access, age string, header bool) D {
	color := colBody
	if header {
		color = colMuted
	}
	cell := func(width unit.Dp, txt string) layout.FlexChild {
		return layout.Rigid(func(gtx C) D {
			l := th.mono(txt, color)
			if header {
				l = th.label(txt, smallSize, color)
			}
			return fixed(gtx, width, func(gtx C) D {
				d := layout.Inset{Right: 4}.Layout(gtx, l.Layout)
				border := clip.Rect{Min: image.Pt(gtx.Constraints.Max.X-1, 0), Max: image.Pt(gtx.Constraints.Max.X, d.Size.Y)}.Push(gtx.Ops)
				paint.Fill(gtx.Ops, colHairSoft)
				border.Pop()
				return d
			})
		})
	}
	return row(gtx, 23, cell(36, address), cell(52, value), cell(64, hex), cell(52, historic), layout.Flexed(1, func(gtx C) D {
		if header {
			return th.label(access, smallSize, color).Layout(gtx)
		}
		return th.mono(access, color).Layout(gtx)
	}), cell(45, age))
}

func packetText(x *Exchange) string {
	sections := []string{}
	for _, p := range []struct {
		name string
		data []byte
	}{{"上游请求", x.UpReq}, {"下游请求", x.DownReq}, {"下游应答", x.DownResp}, {"上游回复", x.UpResp}} {
		msg := fmt.Sprintf("% X", p.data)
		if len(p.data) == 0 {
			msg = packetAbsence(x, p.name)
		}
		sections = append(sections, p.name+"\n"+msg)
	}
	sections = append(sections, fmt.Sprintf("网关请求 PDU\n% X\n\n下游返回 PDU\n% X", x.Request, x.Response))
	return strings.Join(sections, "\n\n")
}

func packetAbsence(x *Exchange, name string) string {
	if strings.HasPrefix(name, "下游") {
		if x.Ds == nil {
			return "没有下游调用（无路由）"
		}
		if x.Ds.InProcess() {
			return "没有线路帧（进程内调用）"
		}
		if name == "下游应答" && x.DownErr != "" {
			return "下游调用失败：" + x.DownErr + "；线路响应未采集"
		}
	}
	return "未采集原始帧"
}

func (v *linkedView) packetPane(gtx C, x *Exchange) D {
	th := v.th
	toggle, copy := v.packetButtons.get(x.Seq), v.copyButtons.get(x.Seq)
	if toggle.Clicked(gtx) {
		v.packets[x.Seq] = !v.packets[x.Seq]
	}
	if copy.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(packetText(x)))})
	}
	children := []layout.FlexChild{layout.Rigid(func(gtx C) D {
		return row(gtx, 32, layout.Rigid(func(gtx C) D {
			txt := "展开已采集报文"
			if v.packets[x.Seq] {
				txt = "收起已采集报文"
			}
			return th.button(gtx, toggle, txt, false)
		}), gap(8), layout.Rigid(func(gtx C) D { return th.button(gtx, copy, "复制已采集报文", false) }))
	})}
	if v.packets[x.Seq] {
		for _, p := range []struct {
			name string
			data []byte
		}{{"网关请求 PDU", x.Request}, {"下游返回 PDU", x.Response}} {
			p := p
			children = append(children, layout.Rigid(th.bold(p.name, smallSize, colBody).Layout))
			for start := 0; start < len(p.data); start += 8 {
				text := fmt.Sprintf("%04X  % X", start, p.data[start:min(start+8, len(p.data))])
				children = append(children, layout.Rigid(th.mono(text, colBody).Layout))
			}
			if len(p.data) == 0 {
				children = append(children, layout.Rigid(th.label("未采集返回 PDU", smallSize, colMuted).Layout))
			}
		}
		for _, p := range []struct {
			name string
			data []byte
		}{{"上游请求", x.UpReq}, {"下游请求", x.DownReq}, {"下游应答", x.DownResp}, {"上游回复", x.UpResp}} {
			p := p
			children = append(children, layout.Rigid(th.bold(fmt.Sprintf("%s · %d 字节", p.name, len(p.data)), smallSize, colBody).Layout))
			if len(p.data) > 0 {
				proto := x.Gw.UpType
				if strings.HasPrefix(p.name, "下游") && x.Ds != nil {
					proto = x.Ds.Type
				}
				req := strings.HasSuffix(p.name, "请求")
				pdu := x.Response
				if req {
					pdu = x.Request
				}
				var parts []string
				for _, f := range frameFields(proto, p.data, pdu, x.Request, req) {
					parts = append(parts, f.label+" "+f.value)
				}
				children = append(children, layout.Rigid(func(gtx C) D {
					l := th.label(strings.Join(parts, " · "), smallSize, colMuted)
					l.MaxLines = 0
					return l.Layout(gtx)
				}))
			}
			if len(p.data) == 0 {
				children = append(children, layout.Rigid(th.label(packetAbsence(x, p.name), smallSize, colMuted).Layout))
				continue
			}
			for start := 0; start < len(p.data); start += 8 {
				start := start
				end := min(start+8, len(p.data))
				children = append(children, layout.Rigid(th.mono(fmt.Sprintf("%04X  % X", start, p.data[start:end]), colBody).Layout))
			}
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}
