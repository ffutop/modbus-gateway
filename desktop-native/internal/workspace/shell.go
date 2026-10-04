// Package workspace implements the approved desktop workspace over live gateway data.
package workspace

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/gateway"
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

	restart, confirm, cancel widget.Clickable
	confirming               bool // the restart warning is showing
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
	if s.restart.Clicked(gtx) && s.canRestart() == "" {
		s.confirming = true
	}
	if s.cancel.Clicked(gtx) {
		s.confirming = false
	}
	if s.confirm.Clicked(gtx) {
		s.confirming = false
		if s.canRestart() == "" {
			s.world.rt.Restart()
		}
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
					layout.Rigid(s.restartControls),
				)
			})
		})
	})
}
func (s *shell) statusBar(gtx C) D {
	return background(gtx, colCard, func(gtx C) D {
		return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
			return fixedH(gtx, 24, func(gtx C) D {
				state, fg := s.runtimeText()
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

// canRestart returns why the gateway cannot be restarted now, or "".
func (s *shell) canRestart() string {
	if s.world.rt.State().Phase == live.Starting {
		return "网关正在启动"
	}
	if s.cfg.unsaved() {
		return "请先保存或撤销配置修改"
	}
	return ""
}

// restartWarning says what a restart interrupts.
func (s *shell) restartWarning() string {
	msg := "重启会中断全部链路的转发"
	for _, sim := range s.world.Sims {
		if strings.HasPrefix(sim.Persist, "memory") {
			msg += "，memory 模型的数据将清空"
			break
		}
	}
	return msg
}

func (s *shell) restartControls(gtx C) D {
	if s.confirming {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(s.th.label(s.restartWarning(), smallSize, colWarn).Layout), gap(8),
			layout.Rigid(func(gtx C) D { return s.th.button(gtx, &s.confirm, "确认重启", true) }), gap(4),
			layout.Rigid(func(gtx C) D { return s.th.button(gtx, &s.cancel, "取消", false) }),
		)
	}
	why := s.canRestart()
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			if why == "" {
				return D{}
			}
			return layout.Inset{Right: 8}.Layout(gtx, s.th.label(why, smallSize, colMuted).Layout)
		}),
		layout.Rigid(func(gtx C) D {
			if why != "" {
				return disabled(gtx, func(gtx C) D { return s.th.button(gtx, &s.restart, "重启网关", false) })
			}
			return s.th.button(gtx, &s.restart, "重启网关", false)
		}),
	)
}

// runtimeText summarizes the gateway process and its listeners.
func (s *shell) runtimeText() (string, color.NRGBA) {
	st := s.world.rt.State()
	switch st.Phase {
	case live.Starting:
		return "正在启动网关…", colMuted
	case live.Stopped:
		if st.Err == nil {
			return "网关未运行 · 可编辑配置", colErr
		}
		return "网关未运行：" + firstLine(st.Err.Error()), colErr
	}
	var ups []gateway.UpstreamStatus
	if s.world.src != nil {
		ups = s.world.src.Upstreams()
	}
	if ups == nil {
		return "运行中 · 正在获取监听状态", colMuted
	}
	listening, starting := 0, 0
	var failed []gateway.UpstreamStatus
	for _, u := range ups {
		switch u.State {
		case gateway.UpstreamListening:
			listening++
		case gateway.UpstreamStarting:
			starting++
		default:
			failed = append(failed, u)
		}
	}
	if len(failed) > 0 {
		f := failed[0]
		return fmt.Sprintf("监听失败 %d/%d · %s 上游 %d：%s", len(failed), len(ups), f.Gateway, f.Index+1, firstLine(f.Error)), colErr
	}
	if starting > 0 {
		return fmt.Sprintf("运行中 · 监听 %d/%d，其余启动中", listening, len(ups)), colMuted
	}
	return fmt.Sprintf("运行中 · 监听 %d/%d", listening, len(ups)), colMuted
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
