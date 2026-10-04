// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package ui draws the desktop window: a toolbar, the gateway sidebar, the
// live request list and an inspector linking decoded fields to raw bytes.
//
// It is immediate mode: every frame is drawn straight from a store snapshot,
// so there is no widget state to keep in sync with the gateways.
package ui

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/store"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// Info describes the running gateway process for the toolbar.
type Info struct {
	ConfigPath string
	Gateways   []string // configured gateway names, in display order
	StartErr   error    // set when the gateways failed to start
}

// UI holds the interaction state; the data lives in the store.
type UI struct {
	th    *Theme
	store *store.Store
	info  Info

	clear      widget.Clickable
	errorsOnly widget.Bool
	gateways   map[string]*widget.Clickable // "" = all gateways

	table     requestTable
	inspector inspector
}

func New(st *store.Store, info Info) *UI {
	u := &UI{
		th:       NewTheme(),
		store:    st,
		info:     info,
		gateways: map[string]*widget.Clickable{},
	}
	u.table.init()
	u.inspector.init()
	return u
}

// Layout draws one frame.
func (u *UI) Layout(gtx C) D {
	u.update(gtx)
	snap := u.store.Read()
	defer snap.Release()

	paint.Fill(gtx.Ops, colCanvas)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return u.toolbar(gtx, snap) }),
		layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }),
		layout.Flexed(1, func(gtx C) D {
			if u.info.StartErr != nil {
				return u.startError(gtx)
			}
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return fixed(gtx, 236, func(gtx C) D { return u.sidebar(gtx, snap) }) }),
				layout.Rigid(func(gtx C) D { return vline(gtx, colHair) }),
				layout.Flexed(1, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D { return u.table.layout(gtx, u.th, snap) }),
						layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }),
						layout.Rigid(func(gtx C) D {
							gtx.Constraints.Min.Y = gtx.Dp(300)
							gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
							return u.inspector.layout(gtx, u.th, u.table.selected())
						}),
					)
				}),
			)
		}),
	)
}

// update applies this frame's input before anything is drawn.
func (u *UI) update(gtx C) {
	if u.clear.Clicked(gtx) {
		u.store.Clear()
	}
	f := u.store.Filter()
	if u.errorsOnly.Update(gtx) {
		f.ErrorsOnly = u.errorsOnly.Value
	}
	for name, btn := range u.gateways {
		if btn.Clicked(gtx) {
			f.Gateway = name
		}
	}
	u.store.SetFilter(f)
}

func (u *UI) toolbar(gtx C, snap store.Snapshot) D {
	th := u.th
	return layout.Inset{Left: 12, Right: 10}.Layout(gtx, func(gtx C) D {
		return row(gtx, 42,
			layout.Rigid(th.bold("ModMux", 14, colInk).Layout),
			gap(10),
			layout.Rigid(th.label(u.info.ConfigPath, smallSize, colMuted).Layout),
			gap(10),
			layout.Rigid(func(gtx C) D {
				if u.info.StartErr != nil {
					return th.badge(gtx, "启动失败", colErr, colErrBg)
				}
				return th.badge(gtx, fmt.Sprintf("运行中 · %d 个网关", len(u.info.Gateways)), colOk, colOkBg)
			}),
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx C) D {
				if snap.Missed() == 0 {
					return D{}
				}
				return layout.Inset{Right: 10}.Layout(gtx, func(gtx C) D {
					return th.badge(gtx, fmt.Sprintf("请求过快，%d 条未显示", snap.Missed()), colWarn, colWarnBg)
				})
			}),
			layout.Rigid(func(gtx C) D {
				cb := material.CheckBox(th.Theme, &u.errorsOnly, "仅看错误")
				cb.Size = 18
				cb.TextSize = textSize
				cb.Color = colBody
				cb.IconColor = colInk
				return cb.Layout(gtx)
			}),
			gap(10),
			layout.Rigid(func(gtx C) D { return th.smallButton(gtx, &u.clear, "清空") }),
		)
	})
}

// smallButton is an outlined button in the design system's style.
func (th *Theme) smallButton(gtx C, btn *widget.Clickable, txt string) D {
	bg := colCanvas
	if btn.Hovered() {
		bg = colSoft
	}
	return btn.Layout(gtx, func(gtx C) D {
		return rounded(gtx, colHair, radiusSm, func(gtx C) D {
			return layout.UniformInset(1).Layout(gtx, func(gtx C) D {
				return rounded(gtx, bg, 5, func(gtx C) D {
					return layout.Inset{Left: 10, Right: 10, Top: 4, Bottom: 4}.Layout(gtx, th.bold(txt, textSize, colInk).Layout)
				})
			})
		})
	})
}

func (u *UI) startError(gtx C) D {
	return layout.Center.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(u.th.bold("网关未能启动", fsTitle, colErr).Layout),
			layout.Rigid(layout.Spacer{Height: 8}.Layout),
			layout.Rigid(func(gtx C) D {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(640))
				l := material.Label(u.th.Theme, textSize, u.info.StartErr.Error())
				l.Color = colBody
				return l.Layout(gtx)
			}),
		)
	})
}

func (u *UI) sidebar(gtx C, snap store.Snapshot) D {
	th := u.th
	filter := u.store.Filter()
	metrics := snap.Metrics()
	children := []layout.FlexChild{
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 14, Top: 12, Bottom: 6}.Layout(gtx, th.bold("网关", smallSize, colMuted).Layout)
		}),
		layout.Rigid(func(gtx C) D {
			var total telemetry.Counts
			var rate store.Rate
			for _, m := range metrics {
				total.Requests += m.Requests
				total.Errors += m.Errors
				r := snap.Rate(m.Name)
				rate.Requests += r.Requests
			}
			return u.gatewayItem(gtx, "", "全部网关", total, rate, filter.Gateway == "")
		}),
	}
	byName := make(map[string]telemetry.Counts, len(metrics))
	for _, m := range metrics {
		byName[m.Name] = m.Counts
	}
	for _, name := range u.info.Gateways {
		children = append(children, layout.Rigid(func(gtx C) D {
			return u.gatewayItem(gtx, name, name, byName[name], snap.Rate(name), filter.Gateway == name)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (u *UI) gatewayItem(gtx C, key, title string, c telemetry.Counts, rate store.Rate, selected bool) D {
	th := u.th
	btn := u.gateways[key]
	if btn == nil {
		btn = new(widget.Clickable)
		u.gateways[key] = btn
	}
	bg := colCanvas
	switch {
	case selected:
		bg = colCard
	case btn.Hovered():
		bg = colSoft
	}
	return layout.Inset{Left: 6, Right: 6, Bottom: 2}.Layout(gtx, func(gtx C) D {
		return btn.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return rounded(gtx, bg, radiusSm, func(gtx C) D {
				return layout.Inset{Left: 8, Right: 8, Top: 6, Bottom: 6}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, th.bold(title, textSize, colInk).Layout),
								layout.Rigid(func(gtx C) D {
									if c.Errors == 0 {
										return D{}
									}
									return th.badge(gtx, fmt.Sprintf("%d 错误", c.Errors), colErr, colErrBg)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: 2}.Layout),
						layout.Rigid(th.mono(fmt.Sprintf("%d 次 · %.1f/s", c.Requests, rate.Requests), colMuted).Layout),
						layout.Rigid(func(gtx C) D {
							if key == "" || c.Requests == 0 {
								return D{}
							}
							return th.mono(fmt.Sprintf("p50 %.2fms · p99 %.2fms", c.P50Ms, c.P99Ms), colFaint).Layout(gtx)
						}),
					)
				})
			})
		})
	})
}
