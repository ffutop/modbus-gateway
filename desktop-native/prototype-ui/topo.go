// PROTOTYPE — throwaway. The topology of one gateway (masters → gateway →
// downstreams → models), drawn above the linked view. Clicking a node
// narrows the link within that gateway.

package main

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

type topoNode struct {
	key   string
	col   int // 0 masters, 1 gateway, 2 downstreams, 3 models
	title string
	sub   string
	ds    *Downstream
	sim   string
	rect  image.Rectangle
}

type topology struct {
	th    *Theme
	world *World
	btns  clicks[string]
}

func (t *topology) nodes(g *Gateway) []*topoNode {
	var ns []*topoNode
	for _, m := range g.Masters {
		ns = append(ns, &topoNode{key: "m/" + m, col: 0, title: m, sub: "主站"})
	}
	ns = append(ns, &topoNode{key: "g", col: 1, title: g.Name + " 网关", sub: g.UpType + " " + g.UpAddr})
	sims := map[string]bool{}
	for _, d := range g.Downstreams {
		sub := fmt.Sprintf("ID %s %s", d.RouteIDs(), d.Type)
		if !d.InProcess() {
			sub += " · " + d.Target
		}
		ns = append(ns, &topoNode{key: "d/" + d.Name, col: 2, title: d.Name, sub: sub, ds: d})
		if d.InProcess() && !sims[d.Sim] {
			sims[d.Sim] = true
			p := ""
			if s := t.world.sim(d.Sim); s != nil {
				p = s.Persist
			}
			ns = append(ns, &topoNode{key: "s/" + d.Sim, col: 3, title: d.Sim, sub: "模拟模型 · " + p, sim: d.Sim})
		}
	}
	return ns
}

// height is how tall the topology of g draws.
func (t *topology) height(g *Gateway) unit.Dp {
	rows := len(g.Masters)
	rows = max(rows, len(g.Downstreams))
	return unit.Dp(36 + 46*rows)
}

// click returns the link after clicking n, within gateway g.
func click(l Link, g *Gateway, n *topoNode) Link {
	switch n.col {
	case 0:
		if l.Master == n.title {
			l.Master = ""
		} else {
			l.Master = n.title
		}
	case 1:
		return Link{Gw: g}
	case 2:
		l.Sim = ""
		if l.Ds == n.ds {
			l.Ds = nil
		} else {
			l.Ds = n.ds
		}
	case 3:
		l.Ds = nil
		if l.Sim == n.sim {
			l.Sim = ""
		} else {
			l.Sim = n.sim
		}
	}
	l.Gw = g
	return l
}

func inLink(l Link, n *topoNode) bool {
	switch n.col {
	case 0:
		return l.Master == "" || l.Master == n.title
	case 1:
		return true
	case 2:
		return (l.Ds == nil || l.Ds == n.ds) && (l.Sim == "" || n.ds.Sim == l.Sim)
	}
	return (l.Sim == "" || l.Sim == n.sim) && (l.Ds == nil || l.Ds.Sim == n.sim)
}

// Layout draws the topology of l.Gw and returns the link after any click.
func (t *topology) Layout(gtx C, l Link) (D, Link) {
	th := t.th
	g := l.Gw
	ns := t.nodes(g)
	for _, n := range ns {
		if t.btns.get(g.Name + "/" + n.key).Clicked(gtx) {
			l = click(l, g, n)
		}
	}
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(t.height(g)))
	colW := size.X / 4
	nodeW, nodeH := min(colW-gtx.Dp(30), gtx.Dp(220)), gtx.Dp(40)
	headH := gtx.Dp(28)
	perCol := map[int][]*topoNode{}
	for _, n := range ns {
		perCol[n.col] = append(perCol[n.col], n)
	}
	for c, list := range perCol {
		step := (size.Y - headH - gtx.Dp(6)) / len(list)
		for i, n := range list {
			x := c*colW + (colW-nodeW)/2
			y := headH + i*step + (step-nodeH)/2
			n.rect = image.Rect(x, y, x+nodeW, y+nodeH)
		}
	}
	find := func(key string) *topoNode {
		for _, n := range ns {
			if n.key == key {
				return n
			}
		}
		return nil
	}
	for c, title := range []string{"上游主站", "网关", "下游", "模拟模型"} {
		off := op.Offset(image.Pt(c*colW+gtx.Dp(14), gtx.Dp(8))).Push(gtx.Ops)
		th.label(title, smallSize, colMuted).Layout(gtx)
		off.Pop()
	}
	edge := func(a, b *topoNode, counts Link, dashed bool) {
		req, errs, rate := t.world.Counts(counts)
		on := inLink(l, a) && inLink(l, b)
		col, width := colLine, float32(gtx.Dp(1.5))

		switch {
		case on && errs*20 > req:
			col = colErrSolid
		case on:
			col = colLineStrong
		}
		if dashed && a.ds != nil && a.ds.Type == "injector" && on {
			col = colAccent
		}
		if on && rate > 0 {
			width += float32(gtx.Dp(unit.Dp(min(rate/6, 3))))
		}
		curve(gtx, a.rect, b.rect, col, width, dashed)
		if on && !dashed && rate > 0 {
			at := image.Pt(a.rect.Max.X+gtx.Dp(6), a.rect.Min.Y+nodeH/2-gtx.Dp(17))
			if a.col == 1 {
				at = image.Pt((a.rect.Max.X+b.rect.Min.X)/2-gtx.Dp(12), b.rect.Min.Y+nodeH/2-gtx.Dp(17))
			}
			off := op.Offset(at).Push(gtx.Ops)
			background(gtx, colCanvas, func(gtx C) D {
				return layout.UniformInset(2).Layout(gtx, th.label(fmt.Sprintf("%.0f/s", rate), microSize, colMuted).Layout)
			})
			off.Pop()
		}
	}
	gn := find("g")
	// Edge counts follow the selected downstream, keeping failures on their own path.
	for _, m := range g.Masters {
		edge(find("m/"+m), gn, Link{Gw: g, Master: m, Ds: l.Ds}, false)
	}
	for _, d := range g.Downstreams {
		dn := find("d/" + d.Name)
		edge(gn, dn, Link{Gw: g, Master: l.Master, Ds: d}, false)
		if d.InProcess() {
			edge(dn, find("s/"+d.Sim), Link{Gw: g, Ds: d}, true)
		}
	}
	for _, n := range ns {
		t.node(gtx, g, n, inLink(l, n) && (l.Master != "" || l.Ds != nil || l.Sim != "" || n.col == 1))
	}
	return D{Size: size}, l
}

func curve(gtx C, a, b image.Rectangle, col color.NRGBA, width float32, dashed bool) {
	p0 := f32.Pt(float32(a.Max.X), float32(a.Min.Y+a.Dy()/2))
	p3 := f32.Pt(float32(b.Min.X), float32(b.Min.Y+b.Dy()/2))
	mid := (p0.X + p3.X) / 2
	c1, c2 := f32.Pt(mid, p0.Y), f32.Pt(mid, p3.Y)
	if !dashed {
		var p clip.Path
		p.Begin(gtx.Ops)
		p.MoveTo(p0)
		p.CubeTo(c1, c2, p3)
		paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
		return
	}
	pt := func(t float32) f32.Point {
		u := 1 - t
		return p0.Mul(u * u * u).Add(c1.Mul(3 * u * u * t)).Add(c2.Mul(3 * u * t * t)).Add(p3.Mul(t * t * t))
	}
	const n = 24
	for i := 0; i < n; i += 2 {
		var p clip.Path
		p.Begin(gtx.Ops)
		p.MoveTo(pt(float32(i) / n))
		p.LineTo(pt(float32(i+1) / n))
		paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
	}
}

func (t *topology) node(gtx C, g *Gateway, n *topoNode, selected bool) {
	th := t.th
	btn := t.btns.get(g.Name + "/" + n.key)
	bg, border, fg := colCanvas, colHair, colBody
	subFg := colMuted
	switch {
	case n.col == 1:
		bg, border, fg, subFg = colDark, colDark, colOnDark, colOnDarkBody
	case selected:
		border, fg = colInk, colInk
	case n.col == 3:
		bg = colSoft
	case btn.Hovered():
		bg, border = colSoft, colLineStrong
	}
	if n.ds != nil && !n.ds.Online {
		border = colErrSolid
	}
	off := op.Offset(n.rect.Min).Push(gtx.Ops)
	g2 := gtx
	g2.Constraints = layout.Exact(n.rect.Size())
	btn.Layout(g2, func(gtx C) D {
		return outlined(gtx, border, bg, radiusLg, func(gtx C) D {
			gtx.Constraints.Min = gtx.Constraints.Max
			return layout.Inset{Left: 9, Right: 9}.Layout(gtx, func(gtx C) D {
				return layout.W.Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									if n.ds == nil {
										return D{}
									}
									col := colOkSolid
									switch {
									case !n.ds.Online:
										col = colErrSolid
									case n.ds.Type == "injector":
										col = colAccent
									}
									return layout.Inset{Right: 5}.Layout(gtx, func(gtx C) D { return dot(gtx, col) })
								}),
								layout.Rigid(th.bold(n.title, smallSize, fg).Layout))
						}),
						layout.Rigid(th.label(n.sub, microSize, subFg).Layout),
					)
				})
			})
		})
	})
	off.Pop()
}
