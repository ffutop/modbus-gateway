package workspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/runlog"
)

var logLevels = []string{"全部级别", "ERROR", "WARN", "INFO", "DEBUG", "UNKNOWN"}
var logSources = []string{"全部来源", "gateway", "desktop"}

type logExportResult struct {
	path string
	err  error
}

type logView struct {
	th                            *Theme
	source                        runlog.Source
	notify                        func()
	query, gateway, path, detail  widget.Editor
	list                          widget.List
	level, origin                 int
	buttons                       clicks[string]
	rows                          clicks[uint64]
	paused, follow                bool
	watermark, version, selected  uint64
	snapshot                      runlog.Snapshot
	shown                         []runlog.Entry
	filter                        string
	lastRefresh                   time.Time
	exporting, writing            bool
	exportText, overwrite, notice string
	result                        chan logExportResult
}

func newLogView(th *Theme, source runlog.Source, notify func()) *logView {
	v := &logView{th: th, source: source, notify: notify, follow: true, buttons: clicks[string]{}, rows: clicks[uint64]{}, result: make(chan logExportResult, 1)}
	v.query.SingleLine, v.gateway.SingleLine, v.path.SingleLine = true, true, true
	v.detail.ReadOnly = true
	v.list.Axis = layout.Vertical
	return v
}

func (v *logView) refresh(now time.Time) {
	if !v.paused && v.source != nil && (v.lastRefresh.IsZero() || now.Sub(v.lastRefresh) >= 100*time.Millisecond) {
		version := v.source.Version()
		if v.lastRefresh.IsZero() || version != v.version {
			v.snapshot = v.source.Snapshot(v.watermark)
			v.version = v.snapshot.Last
			v.filter = "" // invalidate the filter projection
		}
		v.lastRefresh = now
	}
	key := fmt.Sprintf("%d/%d/%s/%s", v.level, v.origin, v.query.Text(), v.gateway.Text())
	if key == v.filter {
		return
	}
	v.filter = key
	v.shown = v.shown[:0]
	q, g := strings.ToLower(v.query.Text()), strings.ToLower(v.gateway.Text())
	valid := map[uint64]bool{}
	for _, e := range v.snapshot.Entries {
		if v.level > 0 && e.Level != logLevels[v.level] {
			continue
		}
		if v.origin > 0 && e.Source != logSources[v.origin] {
			continue
		}
		if g != "" && !strings.Contains(strings.ToLower(e.Gateway), g) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Text()), q) {
			continue
		}
		v.shown = append(v.shown, e)
		valid[e.Seq] = true
	}
	for seq := range v.rows {
		if !valid[seq] {
			delete(v.rows, seq)
		}
	}
	if v.selected != 0 && !valid[v.selected] {
		v.selected = 0
		v.detail.SetText("")
	}
}

func (v *logView) text() string {
	var b strings.Builder
	for _, e := range v.shown {
		b.WriteString(e.Text())
		b.WriteByte('\n')
	}
	return b.String()
}

func (v *logView) selectEntry(e runlog.Entry) {
	v.selected, v.follow = e.Seq, false
	text := fmt.Sprintf("%s · %s · %s · 会话 #%d", e.Time.Format(time.RFC3339Nano), e.Level, sourceName(e.Source), e.Session)
	if e.Gateway != "" {
		text += " · 网关 " + e.Gateway
	}
	text += "\n\n消息\n" + e.Message
	if e.Fields != "" {
		var pretty bytes.Buffer
		fields := e.Fields
		if json.Indent(&pretty, []byte(fields), "", "  ") == nil {
			fields = pretty.String()
		}
		text += "\n\n字段\n" + fields
	}
	text += "\n\n原始记录\n" + e.Raw
	v.detail.SetText(text)
}

func sourceName(s string) string {
	if s == "desktop" {
		return "桌面"
	}
	return "网关"
}

func (v *logView) clear() {
	if v.source != nil {
		v.watermark = v.source.Version()
	}
	v.snapshot.Entries, v.shown = nil, nil
	v.version = v.watermark
	v.selected = 0
	v.detail.SetText("")
	v.rows = clicks[uint64]{}
	v.filter = ""
	v.list.Position = layout.Position{}
}

func (v *logView) export(path string, overwrite bool) {
	v.writing = true
	content := v.exportText
	go func() {
		err := runlog.Export(path, content, overwrite)
		v.result <- logExportResult{path: path, err: err}
		if v.notify != nil {
			v.notify()
		}
	}()
}

func (v *logView) update(gtx C) {
	select {
	case r := <-v.result:
		v.writing = false
		if os.IsExist(r.err) {
			v.overwrite = r.path
			v.notice = "目标文件已存在，再次确认将覆盖：" + r.path
		} else if r.err != nil {
			v.notice = "日志导出失败：" + r.err.Error()
		} else {
			v.exporting = false
			v.overwrite = ""
			v.notice = "已导出：" + r.path
		}
	default:
	}
	if v.buttons.get("level").Clicked(gtx) {
		v.level = (v.level + 1) % len(logLevels)
		v.list.Position = layout.Position{}
	}
	if v.buttons.get("source").Clicked(gtx) {
		v.origin = (v.origin + 1) % len(logSources)
		v.list.Position = layout.Position{}
	}
	if v.buttons.get("pause").Clicked(gtx) {
		v.paused = !v.paused
		if !v.paused {
			v.lastRefresh = time.Time{}
		}
	}
	if v.buttons.get("follow").Clicked(gtx) {
		v.follow = !v.follow
		if v.follow {
			v.paused = false
			v.lastRefresh = time.Time{}
			v.selected = 0
			v.detail.SetText("")
		}
	}
	if v.buttons.get("clear").Clicked(gtx) {
		v.clear()
	}
	v.refresh(gtx.Now)
	for _, e := range v.shown {
		if b := v.rows[e.Seq]; b != nil && b.Clicked(gtx) {
			v.selectEntry(e)
		}
	}
	if v.buttons.get("copy").Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(v.text()))})
		v.notice = fmt.Sprintf("已复制筛选结果：%d 条", len(v.shown))
	}
	if v.buttons.get("copy-selected").Clicked(gtx) && v.selected != 0 {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(v.detail.Text()))})
		v.notice = "已复制所选记录"
	}
	if v.buttons.get("export").Clicked(gtx) && !v.writing {
		v.exporting, v.overwrite = true, ""
		v.exportText = v.text()
		home, _ := os.UserHomeDir()
		v.path.SetText(filepath.Join(home, "modmux-logs-"+time.Now().Format("20060102-150405")+".log"))
		v.notice = "导出当前筛选快照；可修改保存路径"
	}
	if v.buttons.get("cancel-export").Clicked(gtx) && !v.writing {
		v.exporting = false
		v.overwrite = ""
	}
	if v.buttons.get("confirm-export").Clicked(gtx) && !v.writing {
		path := strings.TrimSpace(v.path.Text())
		if path == "" {
			v.notice = "请输入日志导出路径"
			return
		}
		if filepath.Ext(path) == "" {
			path += ".log"
			v.path.SetText(path)
		}
		v.export(path, v.overwrite == path)
	}
}

func (v *logView) Layout(gtx C, cfg *configEditor) D {
	v.update(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 16, Right: 16, Top: 12, Bottom: 8}.Layout(gtx, func(gtx C) D {
				return row(gtx, 34,
					layout.Flexed(1, func(gtx C) D { return cfg.searchField(gtx, &v.query, "搜索消息、字段或会话") }), gap(8),
					layout.Rigid(func(gtx C) D {
						return v.th.button(gtx, v.buttons.get("level"), "级别："+logLevels[v.level], btnDefault)
					}), gap(8),
					layout.Rigid(func(gtx C) D {
						name := logSources[v.origin]
						if v.origin > 0 {
							name = sourceName(name)
						}
						return v.th.button(gtx, v.buttons.get("source"), "来源："+name, btnDefault)
					}), gap(8),
					layout.Rigid(func(gtx C) D {
						return fixed(gtx, 180, func(gtx C) D { return cfg.searchField(gtx, &v.gateway, "筛选网关名称") })
					}),
				)
			})
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 16, Right: 16, Bottom: 8}.Layout(gtx, func(gtx C) D {
				status := fmt.Sprintf("%d / %d 条", len(v.shown), len(v.snapshot.Entries))
				if v.paused {
					status += " · 已暂停"
					if v.source != nil {
						status += fmt.Sprintf(" · 新增 %d 条", v.source.Version()-v.version)
					}
				}
				pause, follow := "暂停", "自动跟随"
				if v.paused {
					pause = "恢复采集视图"
				}
				if v.follow {
					follow = "停止跟随"
				}
				return row(gtx, 30,
					layout.Flexed(1, v.th.label(status, smallSize, colMuted).Layout),
					layout.Rigid(func(gtx C) D { return v.th.button(gtx, v.buttons.get("pause"), pause, btnDefault) }), gap(6),
					layout.Rigid(func(gtx C) D { return v.th.button(gtx, v.buttons.get("follow"), follow, btnDefault) }), gap(6),
					layout.Rigid(func(gtx C) D { return v.th.button(gtx, v.buttons.get("copy"), "复制筛选结果", btnDefault) }), gap(6),
					layout.Rigid(func(gtx C) D { return v.th.button(gtx, v.buttons.get("export"), "导出…", btnDefault) }), gap(6),
					layout.Rigid(func(gtx C) D { return v.th.button(gtx, v.buttons.get("clear"), "清空视图", btnDefault) }),
				)
			})
		}),
		layout.Rigid(func(gtx C) D {
			if !v.exporting {
				return D{}
			}
			return layout.Inset{Left: 16, Right: 16, Bottom: 8}.Layout(gtx, func(gtx C) D {
				return row(gtx, 34,
					layout.Flexed(1, func(gtx C) D {
						if v.writing {
							gtx = gtx.Disabled()
						}
						return cfg.searchField(gtx, &v.path, "保存路径")
					}), gap(8),
					layout.Rigid(func(gtx C) D {
						if v.writing {
							gtx = gtx.Disabled()
						}
						label, kind := "确认导出", btnPrimary
						if v.overwrite == strings.TrimSpace(v.path.Text()) {
							label, kind = "确认覆盖", btnDanger
						}
						return v.th.button(gtx, v.buttons.get("confirm-export"), label, kind)
					}), gap(8),
					layout.Rigid(func(gtx C) D {
						if v.writing {
							gtx = gtx.Disabled()
						}
						return v.th.button(gtx, v.buttons.get("cancel-export"), "取消", btnDefault)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx C) D {
			msg := v.notice
			if v.snapshot.Dropped > 0 {
				msg = fmt.Sprintf("缓存已淘汰 %d 条历史记录 · ", v.snapshot.Dropped) + msg
			}
			if msg == "" {
				return D{}
			}
			color := colMuted
			if strings.HasPrefix(v.notice, "日志导出失败") {
				color = colErr
			}
			if v.overwrite != "" {
				color = colWarn
			}
			label := v.th.label(msg, smallSize, color)
			label.MaxLines = 2
			return layout.Inset{Left: 16, Right: 16, Bottom: 8}.Layout(gtx, label.Layout)
		}),
		layout.Rigid(func(gtx C) D {
			return background(gtx, colSoft, func(gtx C) D { return v.row(gtx, runlog.Entry{}, true) })
		}),
		layout.Flexed(1, func(gtx C) D {
			if len(v.shown) == 0 {
				msg := "暂无运行日志；网关停止后仍可在此查看已采集记录"
				if len(v.snapshot.Entries) > 0 {
					msg = "没有匹配筛选条件的日志"
				}
				return layout.Center.Layout(gtx, v.th.label(msg, textSize, colMuted).Layout)
			}
			v.list.ScrollToEnd = v.follow && !v.paused
			draw := func(gtx C) D {
				return material.List(v.th.Theme, &v.list).Layout(gtx, len(v.shown), func(gtx C, i int) D {
					e := v.shown[i]
					return v.rows.get(e.Seq).Layout(gtx, func(gtx C) D {
						bg := colCanvas
						if e.Seq == v.selected {
							bg = colSelected
						} else if v.rows.get(e.Seq).Hovered() {
							bg = colHover
						}
						return background(gtx, bg, func(gtx C) D { return v.row(gtx, e, false) })
					})
				})
			}
			if !v.list.ScrollToEnd {
				return draw(gtx)
			}
			m := op.Record(gtx.Ops)
			d := draw(gtx)
			call := m.Stop()
			if p := v.list.Position; p.First > 0 || p.Count < len(v.shown) || p.OffsetLast <= 0 {
				call.Add(gtx.Ops)
				return d
			}
			v.list.ScrollToEnd = false
			v.list.Position = layout.Position{}
			d = draw(gtx)
			v.list.ScrollToEnd = true
			return d
		}),
		layout.Rigid(func(gtx C) D {
			if v.selected == 0 {
				return D{}
			}
			return background(gtx, colSoft, func(gtx C) D {
				return layout.Inset{Left: 16, Right: 16, Top: 8, Bottom: 8}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return row(gtx, 28, layout.Rigid(v.th.bold("日志详情", textSize, colInk).Layout), gap(12), layout.Flexed(1, v.th.label("可选择文本 · 滚动查看完整内容", smallSize, colMuted).Layout), layout.Rigid(func(gtx C) D { return v.th.button(gtx, v.buttons.get("copy-selected"), "复制所选", btnDefault) }))
						}),
						layout.Rigid(func(gtx C) D {
							return fixedH(gtx, 220, func(gtx C) D {
								ed := material.Editor(v.th.Theme, &v.detail, "")
								ed.Font, ed.TextSize = monoFont, monoSize
								ed.LineHeight, ed.LineHeightScale = uiLineHeight(monoSize), 1
								return ed.Layout(gtx)
							})
						}),
					)
				})
			})
		}),
	)
}

func (v *logView) row(gtx C, e runlog.Entry, header bool) D {
	values := []string{e.Time.Format("15:04:05.000"), e.Level, sourceName(e.Source), fmt.Sprintf("#%d", e.Session), orDash(e.Gateway), e.Message}
	if e.Gateway == "" {
		values[4] = "—"
	}
	if e.Session == 0 {
		values[3] = "—"
	}
	if header {
		values = []string{"时间", "级别", "来源", "会话", "网关", "消息（点击查看完整记录）"}
	}
	return layout.Inset{Left: 16, Right: 16}.Layout(gtx, func(gtx C) D {
		children := []layout.FlexChild{}
		for i, width := range []unit.Dp{116, 72, 56, 64, 130} {
			i, width := i, width
			children = append(children, layout.Rigid(func(gtx C) D {
				return fixed(gtx, width, func(gtx C) D {
					col := colBody
					if header {
						col = colMuted
					} else if i == 1 {
						if e.Level == "ERROR" {
							col = colErr
						}
						if e.Level == "WARN" {
							col = colWarn
						}
					}
					return v.th.mono(values[i], col).Layout(gtx)
				})
			}))
		}
		children = append(children, layout.Flexed(1, v.th.label(values[5], textSize, colBody).Layout))
		return row(gtx, 32, children...)
	})
}
