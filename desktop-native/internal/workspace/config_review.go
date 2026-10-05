// Package workspace implements the approved desktop workspace over live gateway data.
package workspace

import (
	"fmt"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ffutop/modbus-gateway/internal/config"
	"gopkg.in/yaml.v3"
	"regexp"
	"strconv"
	"strings"
)

func uintSpec(path, label string, p *uint16) *spec {
	return &spec{path: path, label: label, get: func() string { return strconv.Itoa(int(*p)) }, set: func(v string) {
		if n, err := strconv.ParseUint(v, 10, 16); err == nil {
			*p = uint16(n)
		}
	}, check: func(v string) string {
		if _, err := strconv.ParseUint(v, 10, 16); err != nil {
			return "请输入 0–65535 的整数"
		}
		return ""
	}}
}
func yamlAt(n *yaml.Node, path []string) *yaml.Node {
	if n.Kind == yaml.DocumentNode {
		n = n.Content[0]
	}
	if len(path) == 0 {
		return n
	}
	if n.Kind == yaml.SequenceNode {
		i, err := strconv.Atoi(path[0])
		if err == nil && i >= 0 && i < len(n.Content) {
			return yamlAt(n.Content[i], path[1:])
		}
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == path[0] {
			return yamlAt(n.Content[i+1], path[1:])
		}
	}
	return nil
}
func yamlPut(n *yaml.Node, path []string, v *yaml.Node) {
	if n.Kind == yaml.DocumentNode {
		n = n.Content[0]
	}
	if len(path) == 0 {
		head, line, foot := n.HeadComment, n.LineComment, n.FootComment
		*n = *v
		n.HeadComment, n.LineComment, n.FootComment = head, line, foot
		return
	}
	if n.Kind == yaml.SequenceNode {
		i, err := strconv.Atoi(path[0])
		if err != nil {
			return
		}
		for len(n.Content) <= i {
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
		}
		yamlPut(n.Content[i], path[1:], v)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == path[0] {
			yamlPut(n.Content[i+1], path[1:], v)
			return
		}
	}
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if len(path) > 1 {
		if _, err := strconv.Atoi(path[1]); err == nil {
			child.Kind = yaml.SequenceNode
			child.Tag = "!!seq"
		}
	}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: path[0]}, child)
	yamlPut(child, path[1:], v)
}
func (e *configEditor) visualYAML() string {
	source := e.rawSynced
	if source == "" {
		source = e.savedYAML
	}
	var original, canonical yaml.Node
	if yaml.Unmarshal([]byte(source), &original) != nil || yaml.Unmarshal([]byte(toYAML(e.draft)), &canonical) != nil {
		return source
	}
	cfg, err := config.ParseDraft([]byte(source))
	if err != nil || cfg.Version != 1 {
		return source
	}
	var previous yaml.Node
	if yaml.Unmarshal([]byte(toYAML(cfg)), &previous) != nil {
		return source
	}
	for gi, g := range e.draft.Gateways {
		for di, d := range g.Downstreams {
			base := fmt.Sprintf("gateways.%d.downstreams.%d", gi, di)
			parts := strings.Split(base, ".")
			if gi >= len(cfg.Gateways) || di >= len(cfg.Gateways[gi].Downstreams) {
				yamlPut(&original, parts, yamlAt(&canonical, parts))
				continue
			}
			if fmt.Sprint(d.Mappings) != fmt.Sprint(cfg.Gateways[gi].Downstreams[di].Mappings) {
				parts = strings.Split(base+".simulation.mappings", ".")
				v := yamlAt(&canonical, parts)
				if v == nil {
					v = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
				}
				yamlPut(&original, parts, v)
			}
		}
	}
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if !s.visible() {
				continue
			}
			parts := strings.Split(s.path, ".")
			v := yamlAt(&canonical, parts)
			old := yamlAt(&previous, parts)
			if v != nil && (old == nil || old.Value != v.Value) {
				yamlPut(&original, parts, v)
			}
		}
	}
	out, err := yaml.Marshal(&original)
	if err != nil {
		return source
	}
	return string(out)
}
func (e *configEditor) computePendingChanges() []change {
	saved, err := config.ParseDraft([]byte(e.savedYAML))
	if err != nil {
		return nil
	}
	running, err := config.ParseDraft([]byte(e.runningYAML))
	if err != nil {
		return nil
	}
	before := &configEditor{draft: running, ids: copyIDs(e.runningIDs), nextID: e.nextID, eds: map[string]*widget.Editor{}}
	before.rebuild()
	before.commitBaseline()
	after := &configEditor{draft: saved, ids: copyIDs(e.savedIDs), nextID: e.nextID, eds: map[string]*widget.Editor{}}
	after.rebuild()
	after.baseline, after.baseNode, after.baseKinds, after.basePaths = before.baseline, before.baseNode, before.baseKinds, before.basePaths
	out := after.changes()
	if len(out) == 0 && e.savedYAML != e.runningYAML {
		out = append(out, change{label: "文件文本 / 高级配置", old: e.runningYAML, new: e.savedYAML})
	}
	return out
}
func (e *configEditor) downstream(path string) *config.DownstreamConfig {
	parts := strings.Split(path, ".")
	if len(parts) != 4 || parts[2] != "downstreams" {
		return nil
	}
	g, _ := strconv.Atoi(parts[1])
	d, _ := strconv.Atoi(parts[3])
	if g >= len(e.draft.Gateways) || d >= len(e.draft.Gateways[g].Downstreams) {
		return nil
	}
	return &e.draft.Gateways[g].Downstreams[d]
}
func (e *configEditor) updateMappings(gtx C) {
	for path, b := range e.mapAdd {
		if b.Clicked(gtx) {
			if d := e.downstream(path); d != nil {
				e.beginEdit()
				d.Mappings = append(d.Mappings, config.MappingConfig{Source: config.MappingSourceConfig{Table: "holding_registers", Count: 1}, Target: config.MappingTargetConfig{Table: "input_registers"}})
				e.rebuild()
			}
		}
	}
	for path, b := range e.mapRemove {
		if b.Clicked(gtx) {
			i := strings.LastIndex(path, ".")
			idx, _ := strconv.Atoi(path[i+1:])
			if d := e.downstream(path[:i]); d != nil && idx < len(d.Mappings) {
				e.beginEdit()
				d.Mappings = append(d.Mappings[:idx], d.Mappings[idx+1:]...)
				e.rebuild()
			}
		}
	}
}
func (e *configEditor) mappingActions(gtx C, n *cfgNode) D {
	d := e.downstream(n.path)
	if d == nil || d.Type != "injector" {
		return D{}
	}
	var children []layout.FlexChild
	children = append(children, layout.Rigid(func(gtx C) D { return e.configButton(gtx, e.mapAdd.get(n.path), "+ 添加映射", btnDefault) }))
	for i := range d.Mappings {
		children = append(children, gap(6), layout.Rigid(func(gtx C) D {
			return e.configButton(gtx, e.mapRemove.get(fmt.Sprintf("%s.%d", n.path, i)), fmt.Sprintf("删除映射 %d", i+1), btnDanger)
		}))
	}
	return layout.Inset{Left: 24, Bottom: 12}.Layout(gtx, func(gtx C) D { return layout.Flex{}.Layout(gtx, children...) })
}
func (e *configEditor) humanProblem(p config.Problem) string {
	msg := p.Message
	switch {
	case strings.Contains(msg, "is already routed"):
		match := regexp.MustCompile(`slave ID (\d+) is already routed to downstream "([^"]+)"`).FindStringSubmatch(msg)
		if len(match) > 2 {
			return fmt.Sprintf("%s：从站 ID %s 已路由到 %s，请修改本下游的 ID。", e.deepest(problemPath(p)).title(), match[1], match[2])
		}
	case strings.Contains(msg, "invalid slave_ids"):
		return e.deepest(problemPath(p)).title() + "：从站 ID 表达式无效，请使用单值、逗号列表或区间。"
	case strings.Contains(msg, "unknown simulation ref"):
		return e.deepest(problemPath(p)).title() + "：引用的模拟模型不存在，请重新选择。"
	case strings.Contains(msg, "must resolve to exactly one"):
		return e.deepest(problemPath(p)).title() + "：local / injector 必须使用单个从站 ID。"
	case strings.Contains(msg, "overlaps another injector"):
		return e.mappingConflict(p, true)
	case strings.Contains(msg, "overlapping source"):
		return e.mappingConflict(p, false)
	case strings.Contains(msg, "mapping count"):
		return "映射数量必须大于 0"
	case strings.Contains(msg, "mapping table pairing"):
		return "只允许线圈 → 离散输入、保持寄存器 → 输入寄存器"
	case strings.Contains(msg, "out of bounds"):
		return "映射超出地址范围 0–65535：" + msg
	}
	return e.deepest(problemPath(p)).title() + "：" + msg
}
func (e *configEditor) issueList(gtx C) D {
	return layout.Inset{Left: 24, Right: 24, Top: 4, Bottom: 4}.Layout(gtx, func(gtx C) D {
		return material.List(e.th.Theme, &e.issueScroll).Layout(gtx, len(e.problems), func(gtx C, i int) D {
			p := e.problems[i]
			return e.issueBtn.get(problemPath(p)).Layout(gtx, func(gtx C) D {
				label := e.th.label("定位 · "+e.humanProblem(p), smallSize, colErr)
				label.MaxLines = 0
				return layout.Inset{Top: 3, Bottom: 3}.Layout(gtx, func(gtx C) D {
					return rounded(gtx, colErrBg, radiusSm, func(gtx C) D { return layout.UniformInset(8).Layout(gtx, label.Layout) })
				})
			})
		})
	})
}
func (e *configEditor) updateIssues(gtx C) {
	for path, b := range e.issueBtn {
		if !b.Clicked(gtx) {
			continue
		}
		if e.raw {
			if path == "$parse" {
				line, _ := e.parseDiagnostic()
				offset := 0
				for i, text := range strings.Split(e.rawEd.Text(), "\n") {
					if i >= line-1 {
						break
					}
					offset += len([]rune(text)) + 1
				}
				e.rawEd.SetCaret(offset, offset)
				gtx.Execute(key.FocusCmd{Tag: &e.rawEd})
				continue
			}
			var doc yaml.Node
			if yaml.Unmarshal([]byte(e.rawEd.Text()), &doc) == nil {
				node := yamlAt(&doc, strings.Split(path, "."))
				if node != nil {
					lines := strings.Split(e.rawEd.Text(), "\n")
					offset := 0
					for i := 0; i < node.Line-1 && i < len(lines); i++ {
						offset += len([]rune(lines[i])) + 1
					}
					e.rawEd.SetCaret(offset, offset)
					gtx.Execute(key.FocusCmd{Tag: &e.rawEd})
				}
			}
		} else {
			n := e.deepest(path)
			e.navigate(n.path)
			e.wb.advanced[n.id] = true
			if n.gw >= 0 {
				if g := e.node(fmt.Sprintf("gateways.%d", n.gw)); g != nil {
					e.wb.collapsed[g.id] = false
				}
			}
			e.form.Position.First = 0
			index := 1
			for _, s := range n.specs {
				if s.visible() && !strings.Contains(s.path, ".simulation.mappings.") {
					if strings.HasPrefix(s.path, path) || strings.HasPrefix(path, s.path) {
						e.form.Position.First = index
						if ed := e.eds[s.path]; ed != nil {
							gtx.Execute(key.FocusCmd{Tag: ed})
						}
						break
					}
					index++
				}
			}
			if strings.Contains(path, ".simulation.mappings.") {
				e.form.Position.First = index + 2
				for _, s := range n.specs {
					if s.path == path {
						if ed := e.eds[s.path]; ed != nil {
							gtx.Execute(key.FocusCmd{Tag: ed})
						}
						break
					}
				}
			}

		}
	}
}

func (e *configEditor) mappingGrid(gtx C, n *cfgNode) D {
	d := e.downstream(n.path)
	if d == nil || d.Type != "injector" {
		return D{}
	}
	return layout.Inset{Left: 24, Right: 24, Bottom: 16}.Layout(gtx, func(gtx C) D {
		children := []layout.FlexChild{layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }), vgap(16), layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, e.th.bold("写入映射 · 零基地址", textSize, colInk).Layout), layout.Rigid(func(gtx C) D { return e.configButton(gtx, e.mapAdd.get(n.path), "+ 映射", btnDefault) }))
		}), vgap(8), layout.Rigid(e.th.label("地址 0 不等同于设备手册的 40001；结束地址由起点和数量推导。", smallSize, colMuted).Layout), vgap(12)}
		for mi, m := range d.Mappings {
			rangeTitle := fmt.Sprintf("映射 %d", mi+1)
			if m.Source.Count > 0 {
				rangeTitle += fmt.Sprintf(" · %d–%d → %d–%d", m.Source.StartAddress, uint32(m.Source.StartAddress)+uint32(m.Source.Count)-1, m.Target.StartAddress, uint32(m.Target.StartAddress)+uint32(m.Source.Count)-1)
			}
			base := fmt.Sprintf("%s.simulation.mappings.%d.", n.path, mi)
			specs := []*spec{}
			for _, field := range n.specs {
				if strings.HasPrefix(field.path, base) {
					specs = append(specs, field)
				}
			}
			children = append(children, layout.Rigid(func(gtx C) D {
				return outlined(gtx, colHair, colSoft, radiusSm, func(gtx C) D {
					return layout.UniformInset(12).Layout(gtx, func(gtx C) D {
						rows := []layout.FlexChild{layout.Rigid(func(gtx C) D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, e.th.bold(rangeTitle, textSize, colBody).Layout), layout.Rigid(func(gtx C) D {
								return e.configButton(gtx, e.mapRemove.get(fmt.Sprintf("%s.%d", n.path, mi)), "移除", btnDefault)
							}))
						}), vgap(12)}
						for i := 0; i < len(specs); i += 2 {
							left := specs[i]
							if i+1 == len(specs) {
								rows = append(rows, layout.Rigid(func(gtx C) D { return e.fieldRow(gtx, n, left) }))
								continue
							}
							right := specs[i+1]
							rows = append(rows, layout.Rigid(func(gtx C) D {
								return layout.Flex{Alignment: layout.Start}.Layout(gtx, layout.Flexed(1, func(gtx C) D { return e.fieldRow(gtx, n, left) }), gap(24), layout.Flexed(1, func(gtx C) D { return e.fieldRow(gtx, n, right) }))
							}))
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
					})
				})
			}), vgap(12))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (e *configEditor) alignMapping(path, value string) {
	if !strings.HasSuffix(path, ".source.table") {
		return
	}
	prefix := strings.TrimSuffix(path, ".source.table")
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if s.path == prefix+".target.table" {
				if value == "coils" {
					s.set("discrete_inputs")
				} else {
					s.set("input_registers")
				}
			}
		}
	}
}
func (e *configEditor) parseDiagnostic() (int, string) {
	var doc yaml.Node
	line := 1
	match := regexp.MustCompile(`line (\d+)`).FindStringSubmatch(e.rawErr)
	if len(match) > 1 {
		line, _ = strconv.Atoi(match[1])
	}
	description := "YAML 格式或字段类型不正确，请检查第 " + strconv.Itoa(line) + " 行。"
	if yaml.Unmarshal([]byte(e.rawEd.Text()), &doc) == nil {
		var walk func(*yaml.Node, string)
		walk = func(n *yaml.Node, path string) {
			if n.Kind == yaml.DocumentNode {
				for _, c := range n.Content {
					walk(c, path)
				}
				return
			}
			if n.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(n.Content); i += 2 {
					k, v := n.Content[i], n.Content[i+1]
					p := path + "." + k.Value
					if k.Value == "baud_rate" || k.Value == "count" || k.Value == "start_address" {
						if _, err := strconv.Atoi(v.Value); err != nil {
							line = v.Line
							field := map[string]string{"baud_rate": "波特率", "count": "映射数量", "start_address": "起始地址"}[k.Value]
							obj := e.deepest(strings.TrimPrefix(p, "."))
							name := obj.title()
							description = fmt.Sprintf("第 %d 行 · %s / %s：值 %q 应为整数。", line, name, field, v.Value)
							return
						}
					}
					walk(v, p)
				}
			} else {
				for i, c := range n.Content {
					walk(c, fmt.Sprintf("%s.%d", path, i))
				}
			}
		}
		walk(&doc, "")
	}
	return line, description + "\n详细原因：" + e.rawErr
}

// optionChips lays out choices as a segmented control, wrapping onto further
// rows in narrow forms rather than hiding the last option.
func (e *configEditor) mappingConflict(p config.Problem, target bool) string {
	path := problemPath(p)
	node := e.deepest(path)
	ds := e.downstream(node.path)
	marker := ".simulation.mappings."
	at := strings.Index(path, marker)
	if ds == nil || at < 0 {
		return node.title() + "：映射范围冲突。"
	}
	idx, _ := strconv.Atoi(strings.Split(path[at+len(marker):], ".")[0])
	if idx >= len(ds.Mappings) {
		return node.title() + "：映射范围冲突。"
	}
	m := ds.Mappings[idx]
	tableName := m.Source.Table
	start := int(m.Source.StartAddress)
	text := "与本入口其他映射重叠。"
	if target {
		tableName = m.Target.Table
		start = int(m.Target.StartAddress)
		text = "与同一模型的其他注入映射重叠。"
	}
	labels := map[string]string{"holding_registers": "保持寄存器", "coils": "线圈", "input_registers": "输入寄存器", "discrete_inputs": "离散输入"}
	return fmt.Sprintf("%s：映射 %d 的%s @%d–%d %s", node.title(), idx+1, labels[tableName], start, start+int(m.Source.Count)-1, text)
}

func (e *configEditor) issueSummary(gtx C) D {
	if len(e.problems) == 0 {
		return D{}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 16, Right: 16}.Layout(gtx, func(gtx C) D {
				return row(gtx, 32,
					layout.Rigid(func(gtx C) D {
						return e.wbButton(gtx, "show-issues", fmt.Sprintf("%d 处问题", len(e.problems)), btnLink)
					}), gap(8),
					layout.Flexed(1, func(gtx C) D {
						p := e.problems[0]
						return e.issueBtn.get(problemPath(p)).Layout(gtx, func(gtx C) D {
							l := e.th.label("定位 · "+e.humanProblem(p), smallSize, colErr)
							l.MaxLines = 1
							return l.Layout(gtx)
						})
					}))
			})
		}),
		layout.Rigid(func(gtx C) D {
			if !e.showIssues {
				return D{}
			}
			gtx.Constraints.Max.Y = gtx.Dp(96)
			return e.issueList(gtx)
		}))
}

func (e *configEditor) pendingChanges() []change {
	if !e.pendingDiffValid || e.pendingDiffSaved != e.savedYAML || e.pendingDiffRunning != e.runningYAML {
		e.pendingDiff = e.computePendingChanges()
		e.pendingDiffSaved, e.pendingDiffRunning = e.savedYAML, e.runningYAML
		e.pendingDiffValid = true
	}
	out := append([]change(nil), e.pendingDiff...)
	for i := range out {
		if out[i].node != nil {
			if n := e.nodeByID(out[i].node.id); n != nil {
				copyNode := *out[i].node
				copyNode.path = n.path
				out[i].node = &copyNode
			} else {
				out[i].node = nil
			}
		}
	}
	return out
}
