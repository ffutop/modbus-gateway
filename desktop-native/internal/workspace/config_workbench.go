package workspace

import (
	"fmt"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/configfile"
	"github.com/ffutop/modbus-gateway/internal/config"
	"image"
	"strings"
	"time"
)

type navigationPoint struct {
	path, id      string
	first, offset int
}
type gatewayWizard struct {
	id   string
	step int
}
type workbenchState struct {
	wizard               *gatewayWizard
	bulkPaused           bool
	clicks               clicks[string]
	query, pickerQuery   widget.Editor
	filter               string
	filterOpen           bool
	optionPath           string
	collapsed, advanced  map[string]bool
	selected             map[string]*widget.Bool
	back                 []navigationPoint
	picker               string
	overview, pickerList widget.List
	bulk                 *bulkOperation
	notice               string
	lastSelection        string
	entered              time.Time
}

func (e *configEditor) initWorkbench(info Info) {
	e.wb = workbenchState{clicks: clicks[string]{}, filter: "全部", collapsed: map[string]bool{}, advanced: map[string]bool{}, selected: map[string]*widget.Bool{}}
	e.wb.query.SingleLine = true
	e.wb.pickerQuery.SingleLine = true
	e.wb.overview.Axis = layout.Vertical
	e.wb.pickerList.Axis = layout.Vertical
	e.recoveryText = info.Draft
	e.recoveryBase = info.DraftBase
	e.recoveryConflict = info.DraftConflict
	e.saveDraft, e.clearDraft, e.recoveryError = info.SaveDraft, info.ClearDraft, info.DraftError
}
func (e *configEditor) wbButton(gtx C, action, label string, kind btnKind) D {
	draw := func(gtx C) D { return e.configButton(gtx, e.wb.clicks.get(action), label, kind) }
	if (action == "undo" && e.editBase == nil && len(e.history) == 0) || (action == "redo" && (e.editBase != nil || len(e.future) == 0)) || (action == "back" && len(e.wb.back) == 0) {
		return disabled(gtx, draw)
	}
	return draw(gtx)
}
func (e *configEditor) navigate(path string) {
	if e.editBase != nil && !e.pendingRename() {
		explicit := false
		for _, n := range e.nodes {
			for _, s := range n.specs {
				if strings.Contains(s.path, ".simulation.") && e.baseline[e.fieldKey(n, s)] != s.get() {
					explicit = true
				}
			}
		}
		if !explicit {
			if err := e.finishEdits(); err != nil {
				e.structureErr = err.Error()
				return
			}
		}
	}
	if e.sel != path {
		point := navigationPoint{path: e.sel, first: e.form.Position.First, offset: e.form.Position.Offset}
		if n := e.node(e.sel); n != nil {
			point.id = n.id
		}
		e.wb.back = append(e.wb.back, point)
		if len(e.wb.back) > 100 {
			e.wb.back = e.wb.back[1:]
		}
	}
	if e.wb.filter == "无引用" && path != "group:模拟模型" && !strings.HasPrefix(path, "simulations.") {
		e.wb.filter = "全部"
	}
	e.sel = path
	e.wb.optionPath = ""
	e.wb.filterOpen = false
	e.form.Position.First = 0
	e.form.Position.Offset = 0
	e.wb.picker = ""
}
func (e *configEditor) nodeByID(id string) *cfgNode { return e.nodesByID[id] }
func (e *configEditor) dirty(n *cfgNode) bool {
	if e.isNew(n) {
		return true
	}
	for _, s := range n.specs {
		if s.visible() {
			if v, ok := e.baseline[e.fieldKey(n, s)]; !ok || v != s.get() {
				return true
			}
		}
	}
	return false
}
func (e *configEditor) pendingIDs() map[string]bool {
	if e.pendingCache != nil && e.pendingSaved == e.savedYAML && e.pendingRunning == e.runningYAML {
		return e.pendingCache
	}
	m := map[string]bool{}
	if e.savedYAML != e.runningYAML {
		for _, c := range e.pendingChanges() {
			if c.node != nil {
				m[c.node.id] = true
			}
		}
	}
	e.pendingSaved, e.pendingRunning, e.pendingCache = e.savedYAML, e.runningYAML, m
	return m
}
func (e *configEditor) matches(n *cfgNode, pending map[string]bool) bool {
	q := strings.ToLower(strings.TrimSpace(e.wb.query.Text()))
	hay := n.kind + " " + n.title()
	for _, s := range n.specs {
		if s.visible() {
			hay += " " + s.label + " " + s.path + " " + s.get()
		}
	}
	if n.gw >= 0 {
		hay += " " + e.draft.Gateways[n.gw].Name
	}
	if q != "" && !strings.Contains(strings.ToLower(hay), q) {
		return false
	}
	switch e.wb.filter {
	case "有问题":
		if len(e.nodeProblems(n)) > 0 {
			return true
		}
		for _, s := range n.specs {
			if e.fieldErr(s) != "" {
				return true
			}
		}
		return false
	case "未保存":
		return e.dirty(n)
	case "待生效":
		return pending[n.id]
	case "无引用":
		return n.kind == "模拟模型" && len(e.references(n.title())) == 0
	}
	return true
}
func (e *configEditor) status(n *cfgNode) string {
	states := []string{}
	bad := len(e.nodeProblems(n)) > 0
	for _, s := range n.specs {
		if e.fieldErr(s) != "" {
			bad = true
		}
	}
	if bad {
		states = append(states, "有问题")
	}
	if e.dirty(n) {
		states = append(states, "未保存")
	}
	if e.pendingIDs()[n.id] {
		states = append(states, "待生效")
	}
	if len(states) == 0 {
		return "已保存"
	}
	return strings.Join(states, " · ")
}
func (e *configEditor) describe(n *cfgNode) string {
	switch n.kind {
	case "模拟模型":
		for _, s := range e.draft.Simulations {
			if s.Name == n.title() {
				return fmt.Sprintf("%s · %s · %d 条引用", s.Persistence.Type, orDash(s.Persistence.Path), len(e.references(s.Name)))
			}
		}
	case "网关":
		g := e.draft.Gateways[n.gw]
		ids := []string{}
		for _, d := range g.Downstreams {
			ids = append(ids, d.SlaveIDs)
		}
		return fmt.Sprintf("%d 个上游 · %d 个下游 · ID %s", len(g.Upstreams), len(g.Downstreams), strings.Join(ids, " / "))
	case "下游":
		d := e.downstream(n.path)
		target := d.Tcp.Address
		if d.Type == "rtu" {
			target = d.Serial.Device
		}
		if d.Type == "local" || d.Type == "injector" {
			target = d.SimulationRef
		}
		return fmt.Sprintf("%s · ID %s · %s · %d 条映射", d.Type, orDash(d.SlaveIDs), orDash(target), len(d.Mappings))
	case "上游":
		for _, s := range n.specs {
			if s.visible() && (strings.HasSuffix(s.path, ".address") || strings.HasSuffix(s.path, ".device")) {
				return s.get()
			}
		}
	}
	return "全局运行参数"
}
func (e *configEditor) searchField(gtx C, editor *widget.Editor, hint string) D {
	return outlined(gtx, colControl, colCanvas, radiusSm, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Left: 9, Right: 9, Top: 4, Bottom: 4}.Layout(gtx, func(gtx C) D {
			s := material.Editor(e.th.Theme, editor, hint)
			s.TextSize = textSize
			return s.Layout(gtx)
		})
	})
}
func (e *configEditor) workbenchBar(gtx C) D {
	if e.raw {
		return D{}
	}
	return layout.Inset{Left: 20, Right: 20, Top: 10, Bottom: 10}.Layout(gtx, func(gtx C) D {
		return row(gtx, 36,
			layout.Rigid(func(gtx C) D {
				width := unit.Dp(320)
				if gtx.Constraints.Max.X < gtx.Dp(750) {
					width = 220
				}
				return fixed(gtx, width, func(gtx C) D {
					return e.searchField(gtx, &e.wb.query, "搜索对象、地址或字段，如 波特率")
				})
			}), gap(8),
			layout.Rigid(func(gtx C) D {
				label := e.wb.filter
				if label == "全部" {
					label = "全部状态"
				}
				return e.wb.clicks.get("filter-menu").Layout(gtx, func(gtx C) D {
					return outlined(gtx, colControl, colCanvas, radiusSm, func(gtx C) D {
						return layout.Inset{Left: 11, Right: 8, Top: 7, Bottom: 7}.Layout(gtx, func(gtx C) D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Rigid(e.th.label(label, textSize, colBody).Layout), gap(8), layout.Rigid(configChevron))
						})
					})
				})
			}),
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "back", "返回", btnDefault) }), gap(8),
			layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "undo", "撤销", btnDefault) }), gap(8),
			layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "redo", "重做", btnDefault) }))
	})
}
func (e *configEditor) workbenchTree(gtx C) D {
	pending := e.pendingIDs()
	items := []layout.Widget{func(gtx C) D { return e.configNav(gtx, "网关", "网关总览", false) }}
	for _, kind := range []string{"网关", "模拟模型"} {
		items = append(items, func(gtx C) D {
			label := "网关与链路"
			action := "add-gateway"
			if kind == "模拟模型" {
				label = "模拟模型"
				action = "add-simulation"
			}
			return layout.Inset{Left: 18, Right: 20, Top: 12, Bottom: 4}.Layout(gtx, func(gtx C) D {
				return row(gtx, 22, layout.Flexed(1, e.th.label(label, smallSize, colMuted).Layout), layout.Rigid(func(gtx C) D { return e.configPlus(gtx, e.structure.get(action)) }))
			})
		})
		if kind == "模拟模型" {
			items = append(items, func(gtx C) D { return e.configNav(gtx, kind, kind+"总览", true) })
		}

		if e.wb.collapsed[kind] && e.wb.query.Text() == "" {
			continue
		}
		for _, n := range e.nodes {
			if n.kind != kind {
				continue
			}
			show := e.matches(n, pending)
			if kind == "网关" {
				for _, child := range e.nodes {
					if child.depth > 0 && child.gw == n.gw && e.matches(child, pending) {
						show = true
					}
				}
			}
			if !show {
				continue
			}
			items = append(items, func(gtx C) D { return e.treeItem(gtx, n) })
			if kind == "网关" && (!e.wb.collapsed[n.id] || e.wb.query.Text() != "") {
				for _, child := range e.nodes {
					if child.depth > 0 && child.gw == n.gw && e.matches(child, pending) {
						items = append(items, func(gtx C) D { return e.treeItem(gtx, child) })
					}
				}
			}
		}
	}
	if n := e.node("global"); n != nil && e.matches(n, pending) {
		items = append(items, func(gtx C) D {
			return layout.Inset{Left: 18, Top: 16, Bottom: 6}.Layout(gtx, e.th.label("运行设置", smallSize, colMuted).Layout)
		})
		items = append(items, func(gtx C) D { return e.treeItem(gtx, n) })
	}
	return background(gtx, colSoft, func(gtx C) D {
		return layout.Inset{Top: 16, Bottom: 16}.Layout(gtx, func(gtx C) D {
			return e.configList(gtx, &e.tree, len(items), func(gtx C, i int) D { return items[i](gtx) })
		})
	})
}
func (e *configEditor) workbenchPane(gtx C) D {
	if e.wb.picker != "" {
		return e.pickerPane(gtx)
	}
	if e.wb.bulk != nil && !e.wb.bulkPaused {
		return e.bulkPane(gtx)
	}
	if strings.HasPrefix(e.sel, "group:") {
		return e.overviewPane(gtx)
	}
	return e.formPane(gtx)
}
func (e *configEditor) overviewPane(gtx C) D {
	kind := strings.TrimPrefix(e.sel, "group:")
	pending := e.pendingIDs()
	nodes := []*cfgNode{}
	for _, n := range e.nodes {
		if n.kind == kind && e.matches(n, pending) {
			nodes = append(nodes, n)
		}
	}
	widgets := []layout.Widget{func(gtx C) D {
		return layout.Flex{Alignment: layout.Start}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(e.th.bold(kind+"总览", titleSize, colInk).Layout), vgap(4), layout.Rigid(e.th.label("查看配置关系，选择对象后执行批量操作", smallSize, colMuted).Layout))
			}),
			layout.Rigid(func(gtx C) D {
				if len(nodes) == 0 && e.wb.query.Text() == "" && e.wb.filter == "全部" {
					return D{}
				}
				action, label := "add-gateway", "新建网关"
				if kind == "模拟模型" {
					action, label = "add-simulation", "新建模拟模型"
				}
				if kind != "网关" && kind != "模拟模型" {
					return D{}
				}
				return e.configButton(gtx, e.structure.get(action), label, btnDefault)
			}))
	}}

	widgets = append(widgets, func(gtx C) D { return e.configSelectionBar(gtx, nodes) })

	if kind == "下游" {
		widgets = append(widgets, func(gtx C) D { return e.wbButton(gtx, "batch|add", "批量新增下游", btnDefault) })
	}
	if len(nodes) > 0 {
		widgets = append(widgets, func(gtx C) D { return e.configTableHeader(gtx) })
	}
	for _, n := range nodes {
		widgets = append(widgets, func(gtx C) D { return e.configTableRow(gtx, n) })
	}

	if len(nodes) == 0 {
		widgets = append(widgets, func(gtx C) D { return e.configEmptyState(gtx, kind) })
	}

	return layout.Inset{Left: 24, Right: 24, Top: 20, Bottom: 12}.Layout(gtx, func(gtx C) D {
		return e.configList(gtx, &e.wb.overview, len(widgets), func(gtx C, i int) D {
			if i == 0 {
				return layout.Inset{Bottom: 18}.Layout(gtx, widgets[i])
			}
			return widgets[i](gtx)
		})
	})
}
func (e *configEditor) wrapControls(gtx C, children []layout.FlexChild) D {
	if gtx.Constraints.Max.X < gtx.Dp(650) {
		half := (len(children) / 4) * 2
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx C) D { return row(gtx, 34, children[:half]...) }), layout.Rigid(func(gtx C) D { return row(gtx, 34, children[half:]...) }))
	}
	return row(gtx, 34, children...)
}
func (e *configEditor) modelSelector(gtx C, s *spec) D {
	return e.wb.clicks.get("picker|"+s.path).Layout(gtx, func(gtx C) D {
		return outlined(gtx, colControl, colCanvas, radiusSm, func(gtx C) D {
			return layout.Inset{Left: 9, Right: 9, Top: 7, Bottom: 7}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, e.th.label(orDash(s.get()), textSize, colBody).Layout), layout.Rigid(configChevron))
			})
		})
	})
}
func (e *configEditor) pickerPane(gtx C) D {
	items := []layout.Widget{e.th.bold("选择模拟模型", titleSize, colInk).Layout, func(gtx C) D { return e.searchField(gtx, &e.wb.pickerQuery, "搜索模型名称或存储位置") }, func(gtx C) D { return e.wbButton(gtx, "close-picker", "返回原操作", btnDefault) }}
	if strings.HasSuffix(e.wb.picker, ".simulation.ref") {
		items = append(items, func(gtx C) D { return e.wbButton(gtx, "create-model", "新建并引用模型…", btnPrimary) })
	}
	exclude := ""
	if e.deletePath != "" {
		if n := e.node(e.deletePath); n != nil && n.kind == "模拟模型" {
			exclude = n.title()
		}
	}
	matchedModels := 0
	for _, s := range e.draft.Simulations {
		if s.Name == exclude || !strings.Contains(strings.ToLower(s.Name+" "+s.Persistence.Path), strings.ToLower(e.wb.pickerQuery.Text())) {
			continue
		}
		matchedModels++
		items = append(items, func(gtx C) D {
			return e.wbButton(gtx, "pick|"+s.Name, fmt.Sprintf("%s · %s · %d 条引用", s.Name, s.Persistence.Type, len(e.references(s.Name))), btnDefault)
		})
	}
	if matchedModels == 0 {
		items = append(items, e.th.label("没有可选模型。", textSize, colMuted).Layout)
	}
	return layout.Inset{Left: 24, Right: 24, Top: 18}.Layout(gtx, func(gtx C) D {
		return material.List(e.th.Theme, &e.wb.pickerList).Layout(gtx, len(items), func(gtx C, i int) D { return layout.Inset{Bottom: 8}.Layout(gtx, items[i]) })
	})
}
func (e *configEditor) chooseModel(name string) error {
	mode := e.wb.picker
	if mode == "bulk" {
		if e.wb.bulk == nil {
			return fmt.Errorf("批量操作已结束")
		}
		e.wb.bulk.field("simulation.ref", "").SetText(name)
	} else if mode == "replace-all" {
		for _, ref := range e.references(e.node(e.deletePath).title()) {
			e.replacements[ref] = name
		}
	} else if strings.HasPrefix(mode, "replacement:") {
		e.replacements[strings.TrimPrefix(mode, "replacement:")] = name
	} else {
		found := false
		for _, n := range e.nodes {
			for _, s := range n.specs {
				if s.path == mode {
					e.beginEdit()
					s.set(name)
					found = true
				}
			}
		}
		if !found {
			return fmt.Errorf("引用字段已不存在")
		}
	}
	e.wb.picker = ""
	e.version++
	return nil
}
func (e *configEditor) updateWorkbench(gtx C) {
	// Editors outside the currently rendered pane still retain their buffers.
	for _, ed := range []*widget.Editor{&e.wb.query, &e.wb.pickerQuery} {
		for {
			_, ok := ed.Update(gtx)
			if !ok {
				break
			}
		}
	}
	for command, b := range e.wb.clicks {
		if !b.Clicked(gtx) {
			continue
		}
		parts := strings.SplitN(command, "|", 2)
		action, value := parts[0], ""
		if len(parts) > 1 {
			value = parts[1]
		}
		if e.inputBlocked && action != "picker" && action != "pick" && action != "close-picker" && action != "preview-bulk" && action != "confirm-bulk" && action != "cancel-bulk" {
			continue
		}
		var err error
		switch action {
		case "baud":
			parts := strings.SplitN(value, "|", 2)
			if len(parts) == 2 {
				for _, n := range e.nodes {
					for _, s := range n.specs {
						if s.path == parts[0] {
							e.beginEdit()
							s.set(parts[1])
							e.eds[s.path].SetText(parts[1])
							e.version++
						}
					}
				}
			}
		case "show-issues":
			e.showIssues = !e.showIssues
		case "file-path":
			e.showPath = !e.showPath
		case "create-model":
			if strings.HasSuffix(e.wb.picker, ".simulation.ref") {
				e.openCreation("new-model", strings.TrimSuffix(e.wb.picker, ".simulation.ref"))
			}
		case "group":
			e.navigate("group:" + value)
		case "open":
			e.navigate(value)
		case "inspect":
			e.wb.bulkPaused = true
			e.navigate(value)
		case "resume-bulk":
			e.wb.bulkPaused = false
		case "wizard-exit":
			e.wb.wizard = nil
			e.wb.notice = ""
		case "back":
			for len(e.wb.back) > 0 {
				point := e.wb.back[len(e.wb.back)-1]
				e.wb.back = e.wb.back[:len(e.wb.back)-1]
				path := point.path
				if point.id != "" {
					if n := e.nodeByID(point.id); n != nil {
						path = n.path
					} else {
						continue
					}
				}
				e.sel = path
				e.form.Position.First = point.first
				e.form.Position.Offset = point.offset
				e.wb.picker = ""
				break
			}

		case "clear-filter":
			e.wb.query.SetText("")
			e.wb.filter = "全部"
		case "popup-dismiss":
			e.wb.filterOpen = false
			e.wb.optionPath = ""
		case "choice":
			if e.wb.optionPath == value {
				e.wb.optionPath = ""
			} else {
				e.wb.optionPath = value
			}
		case "filter-menu":
			e.wb.filterOpen = !e.wb.filterOpen
		case "filter":
			e.wb.filter = value
			e.wb.filterOpen = false
		case "collapse-id":
			e.wb.collapsed[value] = !e.wb.collapsed[value]
		case "collapse":
			e.wb.collapsed[value] = !e.wb.collapsed[value]
		case "advanced":
			e.wb.advanced[value] = !e.wb.advanced[value]
		case "undo":
			err = e.undoHistory()
		case "redo":
			err = e.redoHistory()
		case "finish":
			err = e.finishEdits()
		case "cancel-edit":
			if e.editBase != nil {
				s := e.editBase
				e.editBase = nil
				err = e.restore(s)
			}
		case "picker":
			e.wb.picker = value
			e.wb.pickerQuery.SetText("")
		case "pick":
			err = e.chooseModel(value)
		case "close-picker":
			e.wb.picker = ""
		case "copy-one":
			e.wb.bulk = newBulk("copy", []string{value})
		case "batch":
			err = e.startBatch(value)
		case "preview-bulk":
			err = e.previewBulk()
		case "confirm-bulk":
			err = e.confirmBulk()
		case "cancel-bulk":
			e.wb.bulk = nil
			e.wb.bulkPaused = false
		case "wizard":
			err = e.startWizard()
		case "wizard-next":
			err = e.wizardNext()
		case "recover":
			err = e.recoverDraft()
		case "discard-recovery":
			e.recoveryText = ""
			if e.clearDraft != nil {
				err = e.clearDraft()
			}
		}
		if err != nil {
			e.structureErr = err.Error()
			e.toast, e.toastAt = err.Error(), gtx.Now
			e.saveFailed = true
		} else {
			e.structureErr = ""
			e.saveFailed = false
		}
		return
	}
}
func (e *configEditor) recoveryBanner(gtx C) D {
	if e.recoveryText == "" {
		if e.recoveryError != nil {
			if err := e.recoveryError(); err != nil {
				return e.th.label("草稿恢复写入失败："+err.Error(), smallSize, colErr).Layout(gtx)
			}
		}
		return D{}
	}
	text := "发现未完成的配置草稿。"
	if e.recoveryConflict {
		text += "正式配置已变化，恢复后请审阅差异。"
	}
	return layout.Inset{Left: 16, Right: 16, Bottom: 8}.Layout(gtx, func(gtx C) D {
		return row(gtx, 34, layout.Flexed(1, e.th.label(text, smallSize, colWarn).Layout), layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "recover", "恢复草稿", btnDefault) }), gap(6), layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "discard-recovery", "丢弃恢复记录", btnDefault) }))
	})
}
func (e *configEditor) recoverDraft() error {
	text := e.recoveryText
	if e.recoveryConflict {
		e.conflict = &configfile.Conflict{Baseline: e.recoveryBase, Disk: e.savedYAML, Draft: text}
		e.conflictView = "草稿"
		e.conflictEditor.SetText(text)
		e.recoveryText = ""
		return nil
	}
	before := e.snapshot()
	cfg, err := config.ParseDraft([]byte(text))
	e.rawEd.SetText(text)
	e.raw = true
	e.rawErr = ""
	if err != nil {
		e.rawErr = err.Error()
	} else {
		e.draft = cfg
		e.reconcileRawIDs()
		e.rawSynced = text
		e.resetControls()
		e.rebuild()
		e.visualLocked = cfg.Version != 1
		e.raw = e.visualLocked
	}
	e.recoveryText = ""
	e.recordEdit(before, "恢复未完成草稿")
	return nil
}

// Use Gio's existing press feedback and deliberate immediate transitions for
// this dense industrial tool; the new pane keeps navigation history visible.

func (e *configEditor) relationsPane(gtx C, n *cfgNode) D {
	items := []layout.FlexChild{}
	if e.wb.notice != "" {
		items = append(items, layout.Rigid(e.th.label(e.wb.notice, textSize, colBody).Layout), vgap(8))
	}
	if n.kind == "网关" {
		items = append(items, layout.Rigid(e.th.label("上下游与路由", smallSize, colMuted).Layout))
		for _, child := range e.nodes {
			if child.gw == n.gw && child.depth > 0 {
				items = append(items, layout.Rigid(func(gtx C) D {
					return e.wbButton(gtx, "open|"+child.path, child.kind+" · "+child.title()+" · "+e.describe(child), btnLink)
				}))
			}
		}
	}
	if d := e.downstream(n.path); d != nil && (d.Type == "local" || d.Type == "injector") {
		for _, sim := range e.nodes {
			if sim.kind == "模拟模型" && sim.title() == d.SimulationRef {
				items = append(items, layout.Rigid(func(gtx C) D {
					return e.wbButton(gtx, "open|"+sim.path, "查看模型 "+sim.title()+" 的全部使用方", btnLink)
				}))
			}
		}
	}
	return layout.Inset{Left: 24, Right: 24, Bottom: 12}.Layout(gtx, func(gtx C) D { return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...) })
}

// Tree expansion uses a drawn directional icon; plus remains exclusive to creation.
func (e *configEditor) treeDisclosure(gtx C, key string, expanded bool) D {
	return e.wb.clicks.get(key).Layout(gtx, func(gtx C) D {
		size := image.Pt(gtx.Dp(12), gtx.Dp(32))
		x, y, r := float32(size.X)/2, float32(size.Y)/2, float32(gtx.Dp(4))
		var p clip.Path
		p.Begin(gtx.Ops)
		if expanded {
			p.MoveTo(f32.Pt(x-r, y-r/2))
			p.LineTo(f32.Pt(x+r, y-r/2))
			p.LineTo(f32.Pt(x, y+r/2))
		} else {
			p.MoveTo(f32.Pt(x-r/2, y-r))
			p.LineTo(f32.Pt(x+r/2, y))
			p.LineTo(f32.Pt(x-r/2, y+r))
		}
		p.Close()
		paint.FillShape(gtx.Ops, colMuted, clip.Outline{Path: p.End()}.Op())
		return D{Size: size, Baseline: gtx.Dp(10)}
	})
}
