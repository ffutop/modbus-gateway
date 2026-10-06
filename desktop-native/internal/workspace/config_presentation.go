package workspace

import (
	"fmt"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"image"
	"image/color"
	"strings"
)

// Configuration presentation follows the frozen A prototype. Business state
// remains in configEditor; these widgets only arrange its public interactions.
func (e *configEditor) configBadge(gtx C, label string, fg, bg color.NRGBA) D {
	gtx.Constraints.Min.X = 0
	return rounded(gtx, bg, radiusXs, func(gtx C) D {
		return layout.Inset{Left: 6, Right: 6, Top: 2, Bottom: 2}.Layout(gtx, e.th.label(label, smallSize, fg).Layout)
	})
}
func (e *configEditor) configNav(gtx C, kind, label string, expand bool) D {
	btn := e.wb.clicks.get("group|" + kind)
	bg, fg := colSoft, colBody
	if e.sel == "group:"+kind {
		bg, fg = colSelected, colOnSelected
	} else if btn.Hovered() {
		bg = colHover
	}
	return layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx C) D {
		return rounded(gtx, bg, radiusSm, func(gtx C) D {
			return layout.Inset{Left: 6, Right: 6}.Layout(gtx, func(gtx C) D {
				return row(gtx, 34, layout.Rigid(func(gtx C) D {
					if expand {
						return e.treeDisclosure(gtx, "collapse|"+kind, !e.wb.collapsed[kind])
					}
					return fixed(gtx, 12, func(gtx C) D { return D{} })
				}), gap(6), layout.Flexed(1, func(gtx C) D {
					return btn.Layout(gtx, func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Rigid(func(gtx C) D { return configRoleIcon(gtx, "总览", fg) }), gap(6), layout.Flexed(1, func(gtx C) D { l := e.th.label(label, textSize, fg); l.MaxLines = 1; return l.Layout(gtx) }))
					})
				}))
			})
		})
	})
}
func (e *configEditor) configTableCells(gtx C, check, name, target, ids, status layout.Widget, height unit.Dp) D {
	return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
		return fixedH(gtx, height, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return fixed(gtx, 44, check) }),
				layout.Flexed(.25, cellWidth(name)), layout.Flexed(.37, cellWidth(target)), layout.Flexed(.20, cellWidth(ids)), layout.Flexed(.18, cellWidth(status)))
		})
	})
}
func (e *configEditor) configTableHeader(gtx C) D {
	return background(gtx, colSoft, func(gtx C) D {
		return e.configTableCells(gtx, func(gtx C) D { return D{} }, e.th.label("名称 / 协议", smallSize, colMuted).Layout, e.th.label("端点 / 模型", smallSize, colMuted).Layout, e.th.label("Slave ID", smallSize, colMuted).Layout, e.th.label("状态", smallSize, colMuted).Layout, 40)
	})
}
func (e *configEditor) configTableRow(gtx C, n *cfgNode) D {
	selected := e.wb.selected[n.id]
	if selected == nil {
		selected = &widget.Bool{}
		e.wb.selected[n.id] = selected
	}
	target, ids, role := e.describe(n), "—", n.kind
	if n.kind == "上游" {
		for _, field := range n.specs {
			if strings.HasSuffix(field.path, ".type") {
				role = field.get()
			}
		}
	}
	if n.kind == "模拟模型" {
		for _, sim := range e.draft.Simulations {
			if sim.Name == n.title() {
				role = sim.Persistence.Type
				target = sim.Persistence.Path
				if target == "" {
					target = "内存"
				}
			}
		}
	}
	if n.kind == "网关" {
		g := e.draft.Gateways[n.gw]
		target = fmt.Sprintf("%d 上游 · %d 下游", len(g.Upstreams), len(g.Downstreams))
	}
	if n.kind == "下游" {
		d := e.downstream(n.path)
		role = d.Type
		ids = d.SlaveIDs
		target = d.Tcp.Address
		if d.Type == "rtu" {
			target = d.Serial.Device
		}
		if d.Type == "local" || d.Type == "injector" {
			target = d.SimulationRef
		}
	}
	bg := colCanvas
	if selected.Value {
		bg = colAccentBg
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx C) D {
		return background(gtx, bg, func(gtx C) D {
			return e.configTableCells(gtx,
				func(gtx C) D {
					s := material.CheckBox(e.th.Theme, selected, "")
					s.Size = unit.Dp(16)
					return s.Layout(gtx)
				},
				func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx C) D {
						return e.wb.clicks.get("open|"+n.path).Layout(gtx, func(gtx C) D {
							l := e.th.bold(configNodeTitle(n), textSize, colPrimary)
							l.MaxLines = 1
							return l.Layout(gtx)
						})
					}), vgap(4), layout.Rigid(e.th.label(role, smallSize, colMuted).Layout))
				},
				func(gtx C) D {
					l := e.th.label(orDash(target), textSize, colBody)
					l.MaxLines = 2
					return l.Layout(gtx)
				},
				func(gtx C) D { return e.configBadge(gtx, orDash(ids), colMuted, colCard) },
				func(gtx C) D {
					fg, bg := colBody, colCard
					status := e.status(n)
					if status != "已保存" {
						fg, bg = colWarn, colWarnBg
					}
					if len(e.nodeProblems(n)) > 0 {
						fg, bg = colErr, colErrBg
					}
					return e.configBadge(gtx, status, fg, bg)
				}, 64)
		})
	}), layout.Rigid(func(gtx C) D { return hline(gtx, colHairSoft) }))
}
func (e *configEditor) configFilterPopup(gtx C) D {
	if !e.wb.filterOpen || e.raw {
		return D{}
	}
	// Overlay under the status selector; it never changes workspace geometry.
	offset := op.Offset(image.Pt(gtx.Dp(348), gtx.Dp(113))).Push(gtx.Ops)
	defer offset.Pop()
	gtx.Constraints.Min = image.Pt(gtx.Dp(150), 0)
	gtx.Constraints.Max = image.Pt(gtx.Dp(150), gtx.Dp(240))
	return outlined(gtx, colControl, colCanvas, radiusSm, func(gtx C) D {
		return layout.UniformInset(6).Layout(gtx, func(gtx C) D {
			children := []layout.FlexChild{}
			for _, f := range []string{"全部", "有问题", "未保存", "待生效", "无引用"} {
				children = append(children, layout.Rigid(func(gtx C) D { return e.configMenuItem(gtx, e.wb.clicks.get("filter|"+f), f, e.wb.filter == f) }), vgap(4))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

func (e *configEditor) configDetailHeader(gtx C, n *cfgNode) D {
	return layout.Inset{Left: 24, Right: 24, Top: 20, Bottom: 18}.Layout(gtx, func(gtx C) D {
		breadcrumb := n.kind
		if n.gw >= 0 && n.kind != "网关" {
			breadcrumb = e.draft.Gateways[n.gw].Name + " / " + n.kind
		}
		title := configNodeTitle(n)
		if n.kind == "常规" {
			title = "全局运行设置"
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(e.th.label(breadcrumb, smallSize, colMuted).Layout), vgap(12),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Start}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(e.th.bold(title, titleSize, colInk).Layout), vgap(4), layout.Rigid(func(gtx C) D {
							subtitle := n.kind
							if n.kind == "网关" {
								subtitle = "按 Slave ID 路由请求"
							}
							if n.kind == "模拟模型" {
								subtitle = "共享模拟数据模型"
							}
							if n.kind == "常规" {
								subtitle = "日志与高级命令行选项"
							}
							return e.th.label(subtitle, smallSize, colMuted).Layout(gtx)
						}))
					}),
					layout.Rigid(func(gtx C) D {
						if n.kind == "常规" {
							return D{}
						}
						return e.wbButton(gtx, "copy-one|"+n.id, "复制", btnDefault)
					}), gap(8),
					layout.Rigid(func(gtx C) D {
						if n.kind == "常规" {
							return D{}
						}
						return e.configButton(gtx, e.structure.get("request-delete|"+n.path), "删除", btnDanger)
					}))
			}),
			layout.Rigid(func(gtx C) D {
				if e.editBase == nil {
					return D{}
				}
				return layout.Inset{Top: 12}.Layout(gtx, func(gtx C) D {
					return row(gtx, 34, layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "finish", "完成编辑", btnPrimary) }), gap(8), layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "cancel-edit", "放弃本次编辑", btnDefault) }))
				})
			}))
	})
}
func (e *configEditor) configFieldGrid(gtx C, n *cfgNode, specs []*spec) D {
	groups := [][]*spec{{}, {}, {}, {}}
	titles := []string{"基本配置", "串口通信", "RS485 专用设置", "高级参数"}
	if n.kind == "常规" {
		titles[0] = "日志"
		titles[3] = "命令行运行设置"
	}
	for _, field := range specs {
		group := 0
		if strings.Contains(field.path, ".serial.") {
			group = 1
			if strings.Contains(field.path, ".rs485") || strings.Contains(field.path, "rts_") || strings.Contains(field.path, ".rx_during_tx") {
				group = 2
			}
		}
		if field.advanced {
			group = 3
		}
		groups[group] = append(groups[group], field)
	}
	children := []layout.FlexChild{}
	for i, fields := range groups {
		if len(fields) == 0 {
			continue
		}
		children = append(children, layout.Rigid(func(gtx C) D { return e.configFieldSection(gtx, n, fields, titles[i]) }))
	}
	for _, field := range n.specs {
		if field.advanced {
			children = append(children, layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: 24, Right: 24, Bottom: 16}.Layout(gtx, func(gtx C) D {
					return e.wbButton(gtx, "advanced|"+n.id, map[bool]string{true: "收起高级参数", false: "高级参数"}[e.wb.advanced[n.id]], btnDefault)
				})
			}))
			break
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}
func (e *configEditor) configFieldSection(gtx C, n *cfgNode, specs []*spec, title string) D {
	return layout.Inset{Left: 24, Right: 24, Bottom: 16}.Layout(gtx, func(gtx C) D {
		width := gtx.Constraints.Max.X
		gtx.Constraints.Max.X = min(width, gtx.Dp(820))
		gtx.Constraints.Min.X = 0
		children := []layout.FlexChild{layout.Rigid(e.th.bold(title, textSize, colInk).Layout), vgap(12)}
		for i := 0; i < len(specs); {
			left := specs[i]
			i++
			wide := strings.HasSuffix(left.path, ".simulation.ref") || strings.HasSuffix(left.path, ".device") || strings.HasSuffix(left.path, ".persistence.path")
			if !wide && i == len(specs) && gtx.Constraints.Max.X >= gtx.Dp(560) {
				children = append(children, layout.Rigid(func(gtx C) D {
					gtx.Constraints.Max.X = (gtx.Constraints.Max.X - gtx.Dp(24)) / 2
					gtx.Constraints.Min.X = 0
					return e.fieldRow(gtx, n, left)
				}))
				continue
			}
			if wide || i == len(specs) || gtx.Constraints.Max.X < gtx.Dp(560) {
				children = append(children, layout.Rigid(func(gtx C) D { return e.fieldRow(gtx, n, left) }))
				continue
			}
			right := specs[i]
			if strings.HasSuffix(right.path, ".simulation.ref") || strings.HasSuffix(right.path, ".device") || strings.HasSuffix(right.path, ".persistence.path") {
				children = append(children, layout.Rigid(func(gtx C) D { return e.fieldRow(gtx, n, left) }))
				continue
			}
			i++
			children = append(children, layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Start}.Layout(gtx, layout.Flexed(1, func(gtx C) D { return e.fieldRow(gtx, n, left) }), gap(24), layout.Flexed(1, func(gtx C) D { return e.fieldRow(gtx, n, right) }))
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}
func (e *configEditor) configRelations(gtx C, n *cfgNode) D {
	return layout.Inset{Left: 24, Right: 24, Bottom: 16}.Layout(gtx, func(gtx C) D {
		children := []layout.FlexChild{}
		kinds := []string{"上游", "下游"}
		if n.kind == "模拟模型" {
			kinds = []string{"使用方"}
		}
		for _, kind := range kinds {
			nodes := []*cfgNode{}
			for _, child := range e.nodes {
				if n.kind == "网关" && child.gw == n.gw && child.kind == kind {
					nodes = append(nodes, child)
				}
				if n.kind == "模拟模型" && child.kind == "下游" {
					d := e.downstream(child.path)
					if d != nil && d.SimulationRef == n.title() {
						nodes = append(nodes, child)
					}
				}
			}
			label := map[string]string{"上游": "主站入口", "下游": "路由覆盖", "使用方": "使用方"}[kind]
			children = append(children, layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }), vgap(16), layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, e.th.bold(fmt.Sprintf("%s · %d", label, len(nodes)), textSize, colInk).Layout), layout.Rigid(func(gtx C) D {
					if kind == "使用方" {
						return D{}
					}
					action := "add-upstream"
					if kind == "下游" {
						action = "add-downstream"
					}
					return e.configButton(gtx, e.structure.get(action+"|"+n.path), "添加"+kind, btnDefault)
				}))
			}), vgap(12))
			if len(nodes) > 0 {
				children = append(children, layout.Rigid(e.configTableHeader))
				for _, child := range nodes {
					children = append(children, layout.Rigid(func(gtx C) D { return e.configTableRow(gtx, child) }))
				}
			} else {
				children = append(children, layout.Rigid(e.th.label("尚无"+label+"。", smallSize, colMuted).Layout))
			}
			children = append(children, vgap(16))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}
func configChevron(gtx C) D {
	size := gtx.Dp(10)
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(1, float32(size)*.35))
	p.LineTo(f32.Pt(float32(size)*.5, float32(size)*.7))
	p.LineTo(f32.Pt(float32(size)-1, float32(size)*.35))
	paint.FillShape(gtx.Ops, colMuted, clip.Stroke{Path: p.End(), Width: float32(gtx.Dp(1))}.Op())
	return D{Size: image.Pt(size, size)}
}
func (e *configEditor) configChoice(gtx C, s *spec) D {
	btns := e.opts[s.path]
	if btns == nil {
		btns = clicks[string]{}
		e.opts[s.path] = btns
	}
	return layout.Stack{}.Layout(gtx, layout.Expanded(func(gtx C) D {
		return e.wb.clicks.get("choice|"+s.path).Layout(gtx, func(gtx C) D {
			return outlined(gtx, colControl, colCanvas, radiusSm, func(gtx C) D {
				return layout.Inset{Left: 9, Right: 9, Top: 7, Bottom: 7}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, e.th.label(s.get(), textSize, colBody).Layout), gap(8), layout.Rigid(configChevron))
				})
			})
		})
	}), layout.Stacked(func(gtx C) D {
		if e.wb.optionPath != s.path {
			return D{}
		}
		macro := op.Record(gtx.Ops)
		offset := op.Offset(image.Pt(0, gtx.Dp(36))).Push(gtx.Ops)
		gtx.Constraints.Min.Y = 0
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		gtx.Constraints.Max.Y = gtx.Dp(300)
		outlined(gtx, colControl, colCanvas, radiusSm, func(gtx C) D {
			children := []layout.FlexChild{}
			for _, v := range s.options {
				children = append(children, layout.Rigid(func(gtx C) D {
					return e.configMenuItem(gtx, btns.get(v), v, s.get() == v)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
		offset.Pop()
		op.Defer(gtx.Ops, macro.Stop())
		return D{}
	}))
}

func cellWidth(w layout.Widget) layout.Widget {
	return func(gtx C) D { width := gtx.Constraints.Max.X; d := w(gtx); d.Size.X = width; return d }
}

func (e *configEditor) configPlus(gtx C, btn *widget.Clickable) D {
	return btn.Layout(gtx, func(gtx C) D {
		return fixed(gtx, 20, func(gtx C) D {
			return fixedH(gtx, 22, func(gtx C) D { return layout.Center.Layout(gtx, e.th.label("+", textSize, colMuted).Layout) })
		})
	})
}
func (e *configEditor) configModuleTab(gtx C, btn *widget.Clickable, label string, on bool) D {
	fg := colMuted
	if on {
		fg = colPrimary
	}
	return btn.Layout(gtx, func(gtx C) D {
		d := layout.Inset{Left: 4, Right: 4, Top: 10, Bottom: 10}.Layout(gtx, func(gtx C) D {
			if on {
				return e.th.bold(label, textSize, fg).Layout(gtx)
			}
			return e.th.label(label, textSize, fg).Layout(gtx)
		})
		if on {
			paint.FillShape(gtx.Ops, colPrimary, clip.Rect{Min: image.Pt(0, d.Size.Y-gtx.Dp(2)), Max: d.Size}.Op())
		}
		return d
	})
}

func (e *configEditor) configMenuItem(gtx C, btn *widget.Clickable, label string, on bool) D {
	bg := colCanvas
	if btn.Hovered() {
		bg = colSoft
	}
	if on {
		bg = colSelected
	}
	return btn.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return background(gtx, bg, func(gtx C) D {
			return layout.Inset{Left: 9, Right: 9, Top: 7, Bottom: 7}.Layout(gtx, e.th.label(label, textSize, colBody).Layout)
		})
	})
}
func (e *configEditor) configPopupDismiss(gtx C) D {
	if !e.wb.filterOpen && e.wb.optionPath == "" {
		return D{}
	}
	return e.wb.clicks.get("popup-dismiss").Layout(gtx, func(gtx C) D { return D{Size: gtx.Constraints.Max} })
}

func (e *configEditor) configList(gtx C, list *widget.List, count int, element layout.ListElement) D {
	style := material.List(e.th.Theme, list)
	style.AnchorStrategy = material.Overlay
	return style.Layout(gtx, count, element)
}

func configNodeTitle(n *cfgNode) string {
	if n.kind == "上游" {
		for _, s := range n.specs {
			if strings.HasSuffix(s.path, ".type") {
				switch s.get() {
				case "tcp":
					return "TCP 主站入口"
				case "rtu":
					return "RTU 主站入口"
				case "rtu-over-tcp":
					return "RTU-over-TCP 入口"
				}
			}
		}
	}
	return n.title()
}
func (e *configEditor) configSelectionBar(gtx C, nodes []*cfgNode) D {
	visible := map[string]bool{}
	for _, n := range nodes {
		visible[n.id] = true
	}
	count, hidden := 0, 0
	onlyDownstream := true
	for id, value := range e.wb.selected {
		if !value.Value {
			continue
		}
		count++
		if !visible[id] {
			hidden++
		}
		if n := e.nodeByID(id); n == nil || n.kind != "下游" {
			onlyDownstream = false
		}
	}
	if count == 0 {
		return D{}
	}
	return layout.Inset{Bottom: 12}.Layout(gtx, func(gtx C) D {
		return background(gtx, colSoft, func(gtx C) D {
			return layout.UniformInset(8).Layout(gtx, func(gtx C) D {
				children := []layout.FlexChild{layout.Rigid(e.th.label(fmt.Sprintf("已选择 %d 项 · 隐藏 %d 项", count, hidden), smallSize, colBody).Layout), gap(12)}
				for _, action := range []struct{ key, label string }{{"copy", "复制"}, {"edit", "批量修改"}, {"move", "移动下游"}, {"delete", "删除"}, {"clear-selection", "取消选择"}} {
					if action.key == "move" && !onlyDownstream {
						continue
					}
					children = append(children, layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "batch|"+action.key, action.label, btnDefault) }), gap(8))
				}
				return e.wrapControls(gtx, children)
			})
		})
	})
}

func (e *configEditor) configButton(gtx C, btn *widget.Clickable, label string, kind btnKind) D {
	bg, fg, border := colCanvas, colBody, colControl
	switch kind {
	case btnPrimary:
		bg, fg, border = colPrimary, colOnPrimary, colPrimary
		if btn.Hovered() {
			bg, border = colPrimaryHover, colPrimaryHover
		}
	case btnDanger:
		fg, border = colErr, colDangerLine
	case btnLink:
		fg, border = colPrimary, colCanvas
	default:
		if btn.Hovered() {
			bg = colSoft
		}
	}
	return btn.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = 0
		return outlined(gtx, border, bg, radiusSm, func(gtx C) D {
			return layout.Inset{Left: 11, Right: 11, Top: 5, Bottom: 5}.Layout(gtx, e.th.label(label, textSize, fg).Layout)
		})
	})
}
func (e *configEditor) configSegment(gtx C, btn *widget.Clickable, txt string, on, first, last bool) D {
	bg, fg := colCanvas, colBody
	switch {
	case on:
		bg, fg = colSelected, colOnSelected
	case btn.Hovered():
		bg = colSoft
	}
	r := gtx.Dp(radiusSm)
	corners := func(rect image.Rectangle, r int) clip.RRect {
		rr := clip.RRect{Rect: rect}
		if first {
			rr.NW, rr.SW = r, r
		}
		if last {
			rr.NE, rr.SE = r, r
		}
		return rr
	}
	return btn.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = 0
		return layout.Background{}.Layout(gtx, func(gtx C) D {
			size := gtx.Constraints.Min
			paint.FillShape(gtx.Ops, colControl, corners(image.Rectangle{Max: size}, r).Op(gtx.Ops))
			left := 0
			if first {
				left = 1
			}
			paint.FillShape(gtx.Ops, bg, corners(image.Rect(left, 1, size.X-1, size.Y-1), max(0, r-1)).Op(gtx.Ops))
			return D{Size: size}
		}, func(gtx C) D {
			left := unit.Dp(10)
			if first {
				left = 11
			}
			return layout.Inset{Left: left, Right: 11, Top: 6, Bottom: 6}.Layout(gtx, e.th.label(txt, textSize, fg).Layout)
		})
	})
}

func (e *configEditor) configToolbarState(gtx C, changes []change, errs int) D {
	children := []layout.FlexChild{}
	badge := func(label string, fg, bg color.NRGBA) {
		children = append(children, layout.Rigid(func(gtx C) D { return e.configBadge(gtx, label, fg, bg) }), gap(6))
	}
	if errs > 0 {
		badge(fmt.Sprintf("%d 处问题", errs), colErr, colErrBg)
	}
	if e.unsaved() {
		badge(fmt.Sprintf("%d 处未保存", max(1, len(changes))), colWarn, colWarnBg)
	}
	if e.needsApply() {
		badge(fmt.Sprintf("%d 处待应用", len(e.pendingChanges())), colWarn, colWarnBg)
	}
	if e.saveFailed {
		badge("保存失败", colErr, colErrBg)
	}
	if e.isStarting() {
		badge("正在启动", colWarn, colWarnBg)
	} else if !e.isRunning() {
		badge("网关已停止", colMuted, colCard)
	}
	if rt, ok := e.runtime.(interface{ ConnectionError() error }); ok && rt.ConnectionError() != nil {
		badge("管理连接中断", colErr, colErrBg)
	}
	if rt, ok := e.runtime.(interface{ UsingRecovery() bool }); ok && rt.UsingRecovery() {
		badge("恢复配置运行中", colWarn, colWarnBg)
	}
	if len(children) == 0 {
		badge("文件与运行一致", colBody, colCard)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (e *configEditor) configEmptyState(gtx C, kind string) D {
	title, subtitle, label, action := "开始配置你的工程", "先创建网关，再添加主站入口和路由目标。", "新建网关", "add-gateway"
	if kind == "模拟模型" {
		title, subtitle, label, action = "还没有模拟模型", "创建共享模型，再由 local 或 injector 下游引用。", "新建模拟模型", "add-simulation"
	}
	filtered := e.wb.query.Text() != "" || e.wb.filter != "全部"
	if filtered {
		title, subtitle, label = "没有匹配对象", "调整搜索词或清除筛选条件。", "清除筛选"
	}
	return fixedH(gtx, 260, func(gtx C) D {
		return layout.Center.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(e.th.bold(title, titleSize, colInk).Layout), vgap(8), layout.Rigid(e.th.label(subtitle, textSize, colMuted).Layout), vgap(16), layout.Rigid(func(gtx C) D {
					if filtered {
						return e.wbButton(gtx, "clear-filter", label, btnPrimary)
					}
					return e.configButton(gtx, e.structure.get(action), label, btnPrimary)
				}))
		})
	})
}
