// Package workspace implements the approved desktop workspace over live gateway data.
package workspace

import (
	"fmt"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"image"
	"image/color"
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/gateway"
)

type shell struct {
	th         *Theme
	world      *World
	cfg        *configEditor
	module     int
	modules    [2]widget.Clickable
	menus      [4]widget.Clickable
	commands   [9]widget.Clickable
	menu       int // zero means closed; otherwise one-based menu index
	dismiss    widget.Clickable
	modalBlock widget.Clickable

	restart, confirm, cancel widget.Clickable
	recoverRun               widget.Clickable
	recovering               bool
	confirming               bool // the restart warning is showing
}

var desktopMenus = [][]string{
	{"编辑配置", "打开 config.yaml", "保存配置"},
	{"查看变更", "撤销未保存修改"},
	{"联动监视", "配置编辑"},
	{"保存配置（重启后生效）", "检查配置文件"},
}

func (s *shell) Layout(gtx C, linked layout.Widget) D {
	modalAtStart := s.modalOpen()
	s.cfg.inputBlocked = modalAtStart
	s.shortcuts(gtx)
	if s.cfg.creation != nil {
		s.cfg.updateCreation(gtx)
	}
	if s.cfg.conflict != nil {
		s.cfg.updateConflict(gtx)
	}
	if s.cfg.deletePath != "" || s.bulkDeleteOpen() {
		s.cfg.updateStructure(gtx)
		s.cfg.updateWorkbench(gtx)
	}
	if !modalAtStart {
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
		if s.cfg.applyRequested {
			s.cfg.applyRequested = false
			if s.canRestart() == "" {
				s.confirming = true
			}
		}
		if s.recoverRun.Clicked(gtx) {
			s.recovering = true
			s.confirming = true
		}
	}
	if s.cancel.Clicked(gtx) {
		s.recovering = false
		s.confirming = false
	}
	if s.confirm.Clicked(gtx) {
		s.confirming = false
		if s.recovering {
			if rt, ok := s.world.rt.(live.RecoveryRuntime); ok && rt.LastGoodConfig() != "" {
				rt.RestartWithConfig(rt.LastGoodConfig())
			}
			s.recovering = false
		} else if s.canRestart() == "" {
			s.world.rt.Restart()
		}
	}
	if !modalAtStart {
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
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			s.cfg.inputBlocked = s.modalOpen()
			if s.modalOpen() {
				gtx = gtx.Disabled()
			}
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
			if s.menu == 0 || s.modalOpen() {
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
		layout.Stacked(s.confirmationDialog),
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
	return background(gtx, colCanvas, func(gtx C) D {
		return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
			return fixedH(gtx, 38, func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return s.cfg.configModuleTab(gtx, &s.modules[0], "联动监视", s.module == 0) }), gap(4),
					layout.Rigid(func(gtx C) D { return s.cfg.configModuleTab(gtx, &s.modules[1], "配置编辑", s.module == 1) }),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx C) D {
						if s.module == 1 {
							return D{}
						}
						return s.restartControls(gtx)
					}),
				)
			})
		})
	})
}
func (s *shell) statusBar(gtx C) D {
	return background(gtx, colSoft, func(gtx C) D {
		return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
			return fixedH(gtx, 28, func(gtx C) D {
				state, fg := s.runtimeText()
				if s.world.missed > 0 {
					state += fmt.Sprintf(" · %d 条未采集", s.world.missed)
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(.6, func(gtx C) D { l := s.th.label(state, smallSize, fg); l.MaxLines = 1; return l.Layout(gtx) }), gap(16),
					layout.Rigid(func(gtx C) D {
						if s.module == 1 {
							return D{}
						}
						return s.th.label(fmt.Sprintf("网关数：%d", len(s.world.Gateways)), smallSize, colMuted).Layout(gtx)
					}),
					gap(16), layout.Flexed(.4, func(gtx C) D {
						label := s.cfg.configPath
						if s.module == 1 {
							label = fmt.Sprintf("草稿 %d 处变更 · 待应用 %d 处 · 网关 %d", len(s.cfg.draftChanges()), len(s.cfg.pendingChanges()), len(s.cfg.draft.Gateways))
						}
						l := s.th.label(label, smallSize, colBody)
						l.MaxLines = 1
						l.Alignment = text.End
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return l.Layout(gtx)
					}),
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
	msg := "重启整个网关进程会中断全部链路的转发（全部网关）"
	if s.recovering {
		msg = "以上次成功配置恢复运行，已保存配置保留"
	}
	for _, sim := range s.world.Sims {
		if strings.HasPrefix(sim.Persist, "memory") {
			msg += "，全部运行中的 memory 模型的数据将清空"
			break
		}
	}
	return msg
}

func (s *shell) restartControls(gtx C) D {

	why := s.canRestart()
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			if rt, ok := s.world.rt.(live.RecoveryRuntime); ok && rt.State().Phase == live.Stopped && rt.LastGoodConfig() != "" {
				return s.th.button(gtx, &s.recoverRun, "恢复上次成功运行", btnDefault)
			}
			return D{}
		}), gap(6),
		layout.Rigid(func(gtx C) D {
			if why == "" {
				return D{}
			}
			return layout.Inset{Right: 8}.Layout(gtx, s.th.label(why, smallSize, colMuted).Layout)
		}),
		layout.Rigid(func(gtx C) D {
			if why != "" {
				return disabled(gtx, func(gtx C) D { return s.th.button(gtx, &s.restart, "重启网关", btnDefault) })
			}
			return s.th.button(gtx, &s.restart, "重启网关", btnDefault)
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
	if rt, ok := s.world.rt.(interface{ ConnectionError() error }); ok && rt.ConnectionError() != nil {
		return "管理连接中断：" + firstLine(rt.ConnectionError().Error()), colErr
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

func (s *shell) bulkDeleteOpen() bool {
	return s.cfg.wb.bulk != nil && s.cfg.wb.bulk.kind == "delete" && !s.cfg.wb.bulkPaused
}
func (s *shell) modalOpen() bool {
	return s.confirming || s.cfg.deletePath != "" || s.bulkDeleteOpen() || s.cfg.creation != nil || s.cfg.conflict != nil
}
func (s *shell) confirmationDialog(gtx C) D {
	if !s.modalOpen() {
		return D{}
	}
	// The full-window click region blocks pointer input without dismissing destructive decisions.
	s.modalBlock.Layout(gtx, func(gtx C) D { return background(gtx, colScrim, func(gtx C) D { return D{Size: gtx.Constraints.Max} }) })
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Center.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X-32, gtx.Dp(520))
		gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y-48, gtx.Dp(480))
		return outlined(gtx, colLine, colCanvas, radiusMd, func(gtx C) D {
			return layout.UniformInset(20).Layout(gtx, func(gtx C) D {
				if s.confirming {
					title, action := "重启网关", "确认重启"
					if s.recovering {
						title, action = "恢复上次成功运行", "确认恢复运行"
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(s.th.bold(title, titleSize, colInk).Layout), vgap(16),
						layout.Rigid(s.th.label(s.restartWarning()+"。", textSize, colBody).Layout), vgap(24),
						layout.Rigid(func(gtx C) D {
							return layout.Flex{}.Layout(gtx,
								layout.Rigid(func(gtx C) D { return s.th.button(gtx, &s.cancel, "取消", btnDefault) }), gap(8),
								layout.Rigid(func(gtx C) D { return s.th.button(gtx, &s.confirm, action, btnPrimary) }))
						}))
				}
				if s.cfg.conflict != nil {
					return s.cfg.conflictPane(gtx)
				}
				if s.cfg.creation != nil {
					return s.cfg.creationPane(gtx)
				}
				if s.bulkDeleteOpen() {
					return s.cfg.bulkPane(gtx)
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Flex{}.Layout(gtx,
							layout.Flexed(1, s.th.bold("删除配置", titleSize, colInk).Layout),
							layout.Rigid(func(gtx C) D {
								return s.cfg.th.button(gtx, s.cfg.structure.get("cancel-delete|"), "取消", btnDefault)
							}))
					}), vgap(12),
					layout.Rigid(func(gtx C) D {
						if s.cfg.wb.picker != "" {
							return s.cfg.pickerPane(gtx)
						}
						s.cfg.dialogList.Axis = layout.Vertical
						return material.List(s.th.Theme, &s.cfg.dialogList).Layout(gtx, 1, func(gtx C, _ int) D { return s.cfg.structureActions(gtx, s.cfg.node(s.cfg.deletePath)) })
					}))
			})
		})
	})
}

func (s *shell) shortcuts(gtx C) {
	filters := []event.Filter{key.Filter{Name: key.NameEscape}}
	if s.modalOpen() {
		filters = append(filters, key.Filter{Name: key.NameTab, Optional: key.ModShift}, key.Filter{Name: key.NameReturn})
	}
	if !s.modalOpen() && s.module == 1 {
		for _, mod := range []key.Modifiers{key.ModCtrl, key.ModCommand} {
			names := []key.Name{"S", "F"}
			if !s.cfg.textFocused(gtx) {
				names = append(names, "Z", "Y")
			}
			for _, name := range names {
				filters = append(filters, key.Filter{Name: name, Required: mod, Optional: key.ModShift})
			}
		}
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		k, ok := ev.(key.Event)
		if !ok || k.State != key.Press {
			continue
		}
		if k.Name == key.NameEscape {
			if s.cfg.wb.filterOpen || s.cfg.wb.optionPath != "" {
				s.cfg.wb.filterOpen = false
				s.cfg.wb.optionPath = ""
			} else if s.cfg.creation != nil {
				s.cfg.creation.clicks.get("cancel").Click()
			} else if s.cfg.conflict != nil {
				s.cfg.conflict = nil
			} else if s.confirming {
				s.cancel.Click()
			} else if s.cfg.deletePath != "" {
				s.cfg.structure.get("cancel-delete|").Click()
			} else if s.bulkDeleteOpen() {
				s.cfg.wb.clicks.get("cancel-bulk").Click()
			} else {
				s.menu = 0
			}
			continue
		}
		if s.modalOpen() {
			if k.Name == key.NameTab {
				s.modalFocus(gtx, k.Modifiers.Contain(key.ModShift))
			}
			if k.Name == key.NameReturn && s.cfg.creation != nil {
				s.cfg.creation.clicks.get("confirm").Click()
			}
			continue
		}
		switch k.Name {
		case "S":
			s.cfg.save.Click()
		case "Z":
			if k.Modifiers.Contain(key.ModShift) {
				s.cfg.wb.clicks.get("redo").Click()
			} else {
				s.cfg.wb.clicks.get("undo").Click()
			}
		case "Y":
			s.cfg.wb.clicks.get("redo").Click()
		case "F":
			if !s.cfg.raw {
				gtx.Execute(key.FocusCmd{Tag: &s.cfg.wb.query})
			}
		}
	}
}

func (e *configEditor) textFocused(gtx C) bool {
	if gtx.Focused(&e.rawEd) || gtx.Focused(&e.wb.query) || gtx.Focused(&e.wb.pickerQuery) {
		return true
	}
	for _, ed := range e.eds {
		if gtx.Focused(ed) {
			return true
		}
	}
	return false
}
func (s *shell) modalFocus(gtx C, back bool) {
	tags := []event.Tag{}
	if c := s.cfg.creation; c != nil {
		tags = append(tags, &c.name)
		if c.action != "add-gateway" {
			for _, v := range []string{"memory", "file", "mmap", "sql"} {
				tags = append(tags, c.clicks.get(v))
			}
			if c.persistence != "memory" {
				tags = append(tags, &c.location)
			}
		}
		tags = append(tags, c.clicks.get("cancel"), c.clicks.get("confirm"))
	} else if s.confirming {
		tags = append(tags, &s.cancel, &s.confirm)
	} else if s.bulkDeleteOpen() {
		b := s.cfg.wb.bulk
		keys := []string{}
		for k := range b.fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			tags = append(tags, b.fields[k])
		}
		tags = append(tags, &b.cascade, s.cfg.wb.clicks.get("preview-bulk"), s.cfg.wb.clicks.get("confirm-bulk"), s.cfg.wb.clicks.get("cancel-bulk"))
	} else if s.cfg.conflict != nil {
		tags = append(tags, &s.cfg.conflictEditor, s.cfg.wb.clicks.get("conflict-cancel"), s.cfg.wb.clicks.get("conflict-copy"), s.cfg.wb.clicks.get("conflict-rebase"))
	} else {
		tags = append(tags, s.cfg.structure.get("cancel-delete|"))
		if s.cfg.wb.picker != "" {
			tags = append(tags, &s.cfg.wb.pickerQuery, s.cfg.wb.clicks.get("close-picker"))
			for _, m := range s.cfg.draft.Simulations {
				tags = append(tags, s.cfg.wb.clicks.get("pick|"+m.Name))
			}
		} else if n := s.cfg.node(s.cfg.deletePath); n != nil {
			if n.kind == "模拟模型" {
				tags = append(tags, s.cfg.wb.clicks.get("picker|replace-all"))
				for _, ref := range s.cfg.references(n.title()) {
					tags = append(tags, s.cfg.wb.clicks.get("picker|replacement:"+ref))
				}
				if len(s.cfg.references(n.title())) > 0 {
					tags = append(tags, s.cfg.structure.get("cascade|"+n.path))
				}
			}
			tags = append(tags, s.cfg.structure.get("delete|"+n.path))
		}
	}
	if len(tags) == 0 {
		return
	}
	next := 0
	for i, t := range tags {
		if gtx.Focused(t) {
			next = (i + 1) % len(tags)
			if back {
				next = (i + len(tags) - 1) % len(tags)
			}
			break
		}
	}
	gtx.Execute(key.FocusCmd{Tag: tags[next]})
}
