// Package workspace implements the approved desktop workspace over live gateway data.
package workspace

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"image"
)

type shell struct {
	th       *Theme
	world    *World
	cfg      *configEditor
	module   int
	modules  [2]widget.Clickable
	menus    [4]widget.Clickable
	commands [9]widget.Clickable
	menu     int // zero means closed; otherwise one-based menu index
	dismiss  widget.Clickable
}

var desktopMenus = [][]string{
	{"编辑配置", "打开 config.yaml", "保存配置"},
	{"查看变更", "撤销未保存修改"},
	{"联动监视", "配置编辑"},
	{"保存配置（重启后生效）", "检查配置文件"},
}

func (s *shell) Layout(gtx C, linked layout.Widget) D {
	for i := range s.modules {
		if s.modules[i].Clicked(gtx) {
			s.module = i
			s.menu = 0
		}
	}
	for i := range s.menus {
		if s.menus[i].Clicked(gtx) {
			if s.menu == i+1 {
				s.menu = 0
			} else {
				s.menu = i + 1
			}
		}
	}
	if s.dismiss.Clicked(gtx) {
		s.menu = 0
	}
	for i := range s.commands {
		if !s.commands[i].Clicked(gtx) {
			continue
		}
		s.menu = 0
		switch i {
		case 0, 6:
			s.module = 1
		case 1:
			s.module = 1
			s.cfg.modes[1].Click()
		case 2:
			s.module = 1
			s.cfg.save.Click()
		case 3:
			s.module = 1
			s.cfg.diff.Click()
		case 4:
			s.module = 1
			s.cfg.revert.Click()
		case 5:
			s.module = 0
		case 7:
			s.module = 1
			s.cfg.save.Click()
		case 8:
			s.module = 1
			s.cfg.modes[1].Click()
		}
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(s.menuBar),
				layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }),
				layout.Rigid(s.toolbar),
				layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }),
				layout.Flexed(1, func(gtx C) D {
					if s.module == 1 {
						return s.cfg.Layout(gtx)
					}
					return linked(gtx)
				}),
				layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }),
				layout.Rigid(s.statusBar),
			)
		}),
		layout.Stacked(func(gtx C) D {
			if s.menu == 0 {
				return D{}
			}
			// Consume clicks outside the popup without delivering them to the workspace.
			s.dismiss.Layout(gtx, func(gtx C) D { return D{Size: gtx.Constraints.Max} })
			off := op.Offset(image.Pt(gtx.Dp(unit.Dp(8+(s.menu-1)*56)), gtx.Dp(29))).Push(gtx.Ops)
			defer off.Pop()
			gtx.Constraints.Min = image.Point{}
			gtx.Constraints.Max.X = gtx.Dp(220)
			base := 0
			for i := 0; i < s.menu-1; i++ {
				base += len(desktopMenus[i])
			}
			return outlined(gtx, colLine, colCanvas, radiusSm, func(gtx C) D {
				return layout.UniformInset(4).Layout(gtx, func(gtx C) D {
					children := []layout.FlexChild{}
					for i, txt := range desktopMenus[s.menu-1] {
						index, txt := base+i, txt
						children = append(children, layout.Rigid(func(gtx C) D {
							return s.commands[index].Layout(gtx, func(gtx C) D {
								bg := colCanvas
								if s.commands[index].Hovered() {
									bg = colHover
								}
								return background(gtx, bg, func(gtx C) D {
									return layout.Inset{Left: 12, Right: 12, Top: 8, Bottom: 8}.Layout(gtx, s.th.label(txt, textSize, colBody).Layout)
								})
							})
						}))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
				})
			})
		}),
	)
}

func (s *shell) menuBar(gtx C) D {
	return background(gtx, colCard, func(gtx C) D {
		return fixedH(gtx, 28, func(gtx C) D {
			children := []layout.FlexChild{gap(8)}
			for i, txt := range []string{"文件", "编辑", "查看", "网关"} {
				i, txt := i, txt
				children = append(children, layout.Rigid(func(gtx C) D {
					return fixed(gtx, 56, func(gtx C) D {
						return s.menus[i].Layout(gtx, func(gtx C) D {
							bg := colCard
							if s.menu == i+1 || s.menus[i].Hovered() {
								bg = colHover
							}
							return background(gtx, bg, func(gtx C) D { return layout.Center.Layout(gtx, s.th.label(txt, textSize, colInk).Layout) })
						})
					})
				}))
			}
			children = append(children, layout.Flexed(1, layout.Spacer{}.Layout), layout.Rigid(s.th.label("ModMux", smallSize, colMuted).Layout), gap(12))
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}
func (s *shell) toolbar(gtx C) D {
	return background(gtx, colSoft, func(gtx C) D {
		return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
			return fixedH(gtx, 40, func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return s.th.tab(gtx, &s.modules[0], "联动监视", s.module == 0) }), gap(4),
					layout.Rigid(func(gtx C) D { return s.th.tab(gtx, &s.modules[1], "配置编辑", s.module == 1) }),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(s.th.label("多网关 · Slave ID 路由", smallSize, colMuted).Layout),
				)
			})
		})
	})
}
func (s *shell) statusBar(gtx C) D {
	return background(gtx, colCard, func(gtx C) D {
		return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
			return fixedH(gtx, 24, func(gtx C) D {
				state, fg := "已启动 · 监听状态见日志", colMuted
				if !s.cfg.running {
					state, fg = "启动失败 · 可编辑配置", colErr
				}
				if s.world.missed > 0 {
					state += fmt.Sprintf(" · %d 条未采集", s.world.missed)
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(s.th.label(state, smallSize, fg).Layout), gap(16),
					layout.Rigid(s.th.label(fmt.Sprintf("网关数：%d", len(s.world.Gateways)), smallSize, colMuted).Layout),
					layout.Flexed(1, layout.Spacer{}.Layout), layout.Rigid(s.th.label(s.cfg.configPath, smallSize, colMuted).Layout),
				)
			})
		})
	})
}
