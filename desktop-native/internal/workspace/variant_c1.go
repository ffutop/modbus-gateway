// Package workspace implements the approved desktop workspace over live gateway data.
package workspace

import (
	"fmt"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type variantC1 struct {
	th     *Theme
	world  *World
	shell  *shell
	linked *linkedView
	topo   *topology
	link   Link
	btns   clicks[Link]
	list   widget.List
}

func newVariantC1(th *Theme, w *World, cfg *configEditor) *variantC1 {
	v := &variantC1{th: th, world: w, shell: &shell{th: th, world: w, cfg: cfg}, linked: newLinkedView(th, w), btns: clicks[Link]{},
		topo: &topology{th: th, world: w, btns: clicks[string]{}}}
	v.list.Axis = layout.Vertical
	if !cfg.running {
		v.shell.module = 1
	}
	if len(w.Gateways) > 0 {
		v.link.Gw = w.Gateways[0]
		if len(w.Gateways[0].Downstreams) > 0 {
			v.link.Ds = w.Gateways[0].Downstreams[0]
		}
	}
	return v
}

func (v *variantC1) Name() string { return "链路列表 + 时序图" }

func (v *variantC1) Layout(gtx C) D {
	return v.shell.Layout(gtx, func(gtx C) D {
		if v.linked.clearMaster {
			v.link.Master = ""
			v.linked.clearMaster = false
		}
		for l, b := range v.btns {
			if b.Clicked(gtx) && l != v.link {
				v.link = l
				v.linked.reset()
			}
		}
		return layout.Flex{}.Layout(gtx,
			layout.Rigid(func(gtx C) D { return fixed(gtx, 270, v.links) }),
			layout.Rigid(func(gtx C) D { return vline(gtx, colHair) }),
			layout.Flexed(1, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						if v.link.Gw == nil {
							return D{}
						}
						d, l := v.topo.Layout(gtx, v.link)
						if l != v.link {
							v.link = l
							v.linked.reset()
						}
						return d
					}),
					layout.Rigid(func(gtx C) D {
						if v.link.Gw == nil {
							return D{}
						}
						return hline(gtx, colHair)
					}),
					layout.Flexed(1, func(gtx C) D { return v.linked.Layout(gtx, v.link, false, v.th.sequence) }),
				)
			}),
		)
	})
}

func (v *variantC1) links(gtx C) D {
	th := v.th
	var items []layout.Widget
	for _, g := range v.world.Gateways {
		items = append(items, func(gtx C) D { return v.linkItem(gtx, Link{Gw: g}, 0) })
		for _, d := range g.Downstreams {
			items = append(items, func(gtx C) D { return v.linkItem(gtx, Link{Gw: g, Ds: d}, 1) })
		}
		items = append(items, layout.Spacer{Height: 8}.Layout)
	}
	items = append(items, th.sectionTitle("模拟模型（跨网关汇总）"))
	for _, s := range v.world.Sims {
		items = append(items, func(gtx C) D { return v.linkItem(gtx, Link{Sim: s.Name}, 0) })
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(th.sectionTitle("链路")),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14, Right: 10, Bottom: 6}.Layout(gtx, th.label("灰色未观测 · 红绿为最近请求结果", microSize, colMuted).Layout)
		}),
		layout.Flexed(1, func(gtx C) D {
			return material.List(th.Theme, &v.list).Layout(gtx, len(items), func(gtx C, i int) D { return items[i](gtx) })
		}),
	)
}

func (v *variantC1) linkItem(gtx C, l Link, depth int) D {
	th := v.th
	btn := v.btns.get(l)
	bg := colCanvas
	// The topology may narrow further (master, model); the list row stays
	// selected for the gateway or downstream it belongs to.
	cur := v.link
	cur.Master = ""
	if cur.Gw != nil && cur.Sim != "" {
		cur.Sim = ""
	}
	fg, secondary := colBody, colMuted
	switch {
	case l == cur:
		bg, fg, secondary = colSelected, colOnSelected, colBody
	case btn.Hovered():
		bg = colSoft
	}
	req, errs, rate := v.world.Counts(l)
	return layout.Inset{Left: 6, Right: 6}.Layout(gtx, func(gtx C) D {
		return btn.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return rounded(gtx, bg, 0, func(gtx C) D {
				return layout.Inset{Left: 8 + unit.Dp(depth)*14, Right: 8, Top: 5, Bottom: 5}.Layout(gtx, func(gtx C) D {
					switch {
					case l.Sim != "":
						return v.simItemColors(gtx, l, fg, secondary)
					case l.Ds == nil:
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
									layout.Flexed(1, th.bold(l.Gw.Name+" 网关", textSize, fg).Layout),
									layout.Rigid(th.mono(fmt.Sprintf("%.0f/s", rate), secondary).Layout))
							}),
							layout.Rigid(th.label(fmt.Sprintf("上游 %s %s · %d 个主站", l.Gw.UpType, l.Gw.UpAddr, len(l.Gw.Masters)), smallSize, secondary).Layout),
						)
					}
					d := l.Ds
					status := colFaint
					switch {
					case d.Observed && !d.Online:
						status = colErrSolid
					case d.Observed && d.Online:
						status = colOkSolid
					case errs*20 > req:
						status = colWarnSolid
					}
					sub := fmt.Sprintf("ID %s · %s · %s", d.RouteIDs(), d.Type, d.Target)
					if d.InProcess() {
						sub = fmt.Sprintf("ID %s · %s → 模型 %s", d.RouteIDs(), d.Type, d.Sim)
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									if d.InProcess() {
										return th.badge(gtx, "模型", colBody, colCard)
									}
									return dot(gtx, status)
								}),
								gap(6),
								layout.Flexed(1, th.label(d.Name, textSize, fg).Layout),
								layout.Rigid(func(gtx C) D {
									if errs == 0 {
										return th.mono(fmt.Sprintf("%.0f/s", rate), secondary).Layout(gtx)
									}
									return th.badge(gtx, fmt.Sprintf("%.0f/s · %d 错", rate, errs), colErr, colErrBg)
								}))
						}),
						layout.Rigid(th.label(sub, smallSize, secondary).Layout),
					)
				})
			})
		})
	})
}

func (v *variantC1) simItem(gtx C, l Link) D {
	return v.simItemColors(gtx, l, colInk, colMuted)
}
func (v *variantC1) simItemColors(gtx C, l Link, fg, secondary color.NRGBA) D {
	th := v.th
	var refs []string
	for _, g := range v.world.Gateways {
		for _, d := range g.Downstreams {
			if d.Sim == l.Sim {
				refs = append(refs, d.Name)
			}
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return th.badge(gtx, "模型", colBody, colCard) }),
				gap(6),
				layout.Flexed(1, th.bold(l.Sim, textSize, fg).Layout))
		}),
		layout.Rigid(th.label(fmt.Sprintf("被 %d 个下游引用：%v", len(refs), refs), smallSize, secondary).Layout),
	)
}
