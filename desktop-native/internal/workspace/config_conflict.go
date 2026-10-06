package workspace

import (
	"errors"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ffutop/modbus-gateway/internal/config"
	"io"
	"strings"
)

func (e *configEditor) updateConflict(gtx C) {
	if e.conflict == nil {
		return
	}
	for {
		if _, ok := e.conflictEditor.Update(gtx); !ok {
			break
		}
	}
	for _, view := range []string{"基线", "磁盘", "草稿"} {
		if e.wb.clicks.get("conflict|" + view).Clicked(gtx) {
			e.conflictView = view
			e.conflictEditor.SetText(map[string]string{"基线": e.conflict.Baseline, "磁盘": e.conflict.Disk, "草稿": e.conflict.Draft}[view])
		}
	}
	if e.wb.clicks.get("conflict-copy").Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(e.conflict.Draft))})
	}
	if e.wb.clicks.get("conflict-cancel").Clicked(gtx) {
		e.conflict = nil
		return
	}
	if e.wb.clicks.get("conflict-rebase").Clicked(gtx) && e.rebaseFile != nil {
		reviewed := e.conflict
		if err := e.rebaseFile(reviewed.Disk); err != nil {
			if errors.As(err, &e.conflict) {
				e.conflict.Draft = reviewed.Draft
				e.conflictView = "磁盘"
				e.conflictEditor.SetText(e.conflict.Disk)
			}
			e.structureErr = err.Error()
			return
		}
		// Read the new disk baseline on a separate editor so the draft and history survive.
		if cfg, err := config.ParseDraft([]byte(reviewed.Disk)); err == nil {
			baseline := &configEditor{draft: cfg, rawSynced: e.savedYAML, ids: copyIDs(e.savedIDs), nextID: e.nextID, eds: map[string]*widget.Editor{}}
			baseline.reconcileRawIDs()
			baseline.rebuild()
			baseline.commitBaseline()
			e.nextID = baseline.nextID
			e.baseline, e.baseNode, e.baseKinds, e.basePaths = baseline.baseline, baseline.baseNode, baseline.baseKinds, baseline.basePaths
			e.savedIDs = copyIDs(baseline.ids)
		}
		e.savedYAML = reviewed.Disk
		e.raw = true
		e.rawEd.SetText(reviewed.Draft)
		e.reparse()
		e.conflict = nil
		e.saveFailed = false
		e.saveProblem = ""
		e.structureErr = "已将磁盘版本设为基线，草稿完整保留；请在 YAML 手动合并，再校验保存。"
	}
}
func (e *configEditor) conflictPane(gtx C) D {
	e.conflictEditor.ReadOnly = true
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(e.th.bold("配置文件冲突", titleSize, colInk).Layout), vgap(8),
		layout.Rigid(e.th.label("文件未被覆盖。查看基线、磁盘和草稿，复制草稿或重新基准化后手动合并。", smallSize, colBody).Layout), vgap(8),
		layout.Rigid(func(gtx C) D {
			if e.structureErr == "" {
				return D{}
			}
			return e.th.label(e.structureErr, smallSize, colErr).Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			parts := []layout.FlexChild{}
			for _, v := range []string{"基线", "磁盘", "草稿"} {
				parts = append(parts, layout.Rigid(func(gtx C) D { return e.th.tab(gtx, e.wb.clicks.get("conflict|"+v), v, e.conflictView == v) }), gap(6))
			}
			return row(gtx, 34, parts...)
		}),
		layout.Flexed(1, func(gtx C) D {
			gtx.Constraints.Min = gtx.Constraints.Max
			ed := material.Editor(e.th.Theme, &e.conflictEditor, "")
			ed.Font = monoFont
			ed.TextSize = monoSize
			return ed.Layout(gtx)
		}), vgap(12),
		layout.Rigid(func(gtx C) D {
			return row(gtx, 34,
				layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "conflict-cancel", "返回草稿", btnDefault) }), gap(6),
				layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "conflict-copy", "复制草稿", btnDefault) }), gap(6),
				layout.Rigid(func(gtx C) D {
					draw := func(gtx C) D { return e.wbButton(gtx, "conflict-rebase", "以磁盘为基线合并", btnPrimary) }
					if e.rebaseFile == nil {
						return disabled(gtx, draw)
					}
					return draw(gtx)
				}))
		}))
}
