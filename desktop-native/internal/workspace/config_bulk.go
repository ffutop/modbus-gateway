package workspace

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/routing"
	"gopkg.in/yaml.v3"
	"sort"
	"strconv"
	"strings"
)

type bulkOperation struct {
	requestRevision      string
	kind                 string
	ids                  []string
	fields               map[string]*widget.Editor
	independent, cascade widget.Bool
	preview              *structureSnapshot
	revision             string
	summary              []string
	problems             []config.Problem
	list                 widget.List
	wizardStep           int
}

func newBulk(kind string, ids []string) *bulkOperation {
	b := &bulkOperation{kind: kind, ids: ids, fields: map[string]*widget.Editor{}}
	b.list.Axis = layout.Vertical
	return b
}
func (b *bulkOperation) field(key, value string) *widget.Editor {
	if b.fields[key] == nil {
		b.fields[key] = &widget.Editor{SingleLine: true}
		b.fields[key].SetText(value)
	}
	return b.fields[key]
}
func (b *bulkOperation) value(key string) string {
	if b.fields[key] == nil {
		return ""
	}
	return strings.TrimSpace(b.fields[key].Text())
}
func (e *configEditor) selectedIDs() []string {
	ids := []string{}
	for _, n := range e.nodes {
		if b := e.wb.selected[n.id]; b != nil && b.Value {
			ids = append(ids, n.id)
		}
	}
	return ids
}
func (e *configEditor) startBatch(action string) error {
	if action == "select-visible" {
		pending := e.pendingIDs()
		kind := strings.TrimPrefix(e.sel, "group:")
		for _, n := range e.nodes {
			if n.kind == kind && e.matches(n, pending) {
				if e.wb.selected[n.id] == nil {
					e.wb.selected[n.id] = &widget.Bool{}
				}
				e.wb.selected[n.id].Value = true
			}
		}
		return nil
	}
	if action == "clear-selection" {
		e.wb.selected = map[string]*widget.Bool{}
		return nil
	}
	if err := e.finishEdits(); err != nil {
		return err
	}
	ids := e.selectedIDs()
	if action != "add" && len(ids) == 0 {
		return fmt.Errorf("请先勾选对象")
	}
	e.wb.bulk = newBulk(action, ids)
	return nil
}
func nodeText(n *yaml.Node) (string, error) { b, err := yaml.Marshal(n); return string(b), err }
func sortedPaths(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		a, b := strings.Split(paths[i], "."), strings.Split(paths[j], ".")
		for k := 0; k < len(a) && k < len(b); k++ {
			if a[k] == b[k] {
				continue
			}
			x, xe := strconv.Atoi(a[k])
			y, ye := strconv.Atoi(b[k])
			if xe == nil && ye == nil {
				return x > y
			}
			return a[k] > b[k]
		}
		return len(a) > len(b)
	})
}
func fieldValue(root *yaml.Node, path string) *yaml.Node {
	return yamlAt(root, strings.Split(path, "."))
}
func (e *configEditor) previewBulk() error {
	b := e.wb.bulk
	if b == nil {
		return nil
	}
	if err := e.finishEdits(); err != nil {
		return err
	}
	before := e.snapshot()
	root, err := parseNode(before.text)
	if err != nil {
		return err
	}
	ids := copyIDs(e.ids)
	b.summary = nil
	b.preview = nil
	b.problems = nil
	nodes := []*cfgNode{}
	for _, id := range b.ids {
		n := e.nodeByID(id)
		if n == nil {
			return fmt.Errorf("所选对象已变化，请重新选择")
		}
		nodes = append(nodes, n)
	}
	// Field overrides apply to the original detached nodes before copy or move.
	apply := func(n *yaml.Node, index int) error {
		upstream := yamlAt(n, []string{"name"}) == nil
		container := yamlAt(n, []string{"upstreams"}) != nil || yamlAt(n, []string{"persistence"}) != nil
		for _, key := range []string{"type", "tcp.address", "serial.device", "serial.baud_rate", "simulation.ref", "slave_ids"} {
			if container || upstream && (key == "simulation.ref" || key == "slave_ids") {
				continue
			}
			if v := b.value(key); v != "" {
				node := scalar(v)
				if key == "serial.baud_rate" {
					if _, err := strconv.Atoi(v); err != nil {
						return fmt.Errorf("波特率必须是整数")
					}
					node.Tag = "!!int"
				}
				yamlPut(n, strings.Split(key, "."), node)
			}
		}
		if prefix := b.value("name_prefix"); prefix != "" && !upstream {
			yamlPut(n, []string{"name"}, scalar(fmt.Sprintf("%s-%d", prefix, index+1)))
		}
		if start := b.value("slave_start"); start != "" && !upstream && !container {
			v, err := strconv.Atoi(start)
			if err != nil || v < 1 || v+index > 247 {
				return fmt.Errorf("连续从站 ID 应在 1–247 范围内")
			}
			yamlPut(n, []string{"slave_ids"}, scalar(strconv.Itoa(v+index)))
		}
		return nil
	}
	switch b.kind {
	case "edit":
		for i, n := range nodes {
			if n.kind != "下游" && n.kind != "上游" {
				return fmt.Errorf("批量修改支持上游和下游；模型或网关请进入详情")
			}
			if err := apply(fieldValue(root, n.path), i); err != nil {
				return err
			}
			b.summary = append(b.summary, "修改 "+n.title())
		}
	case "add":
		gi, err := strconv.Atoi(b.value("gateway"))
		if err != nil || gi < 1 || gi > len(e.draft.Gateways) {
			return fmt.Errorf("请选择有效的目标网关序号")
		}
		gi--
		count, err := strconv.Atoi(b.value("count"))
		if err != nil || count < 1 || count > 247 {
			return fmt.Errorf("新增数量应在 1–247 范围内")
		}
		g := e.draft.Gateways[gi]
		if len(g.Downstreams) == 1 && g.Downstreams[0].SlaveIDs == "" {
			return fmt.Errorf("请先为原默认下游配置 Slave ID")
		}
		if b.value("slave_start") == "" {
			return fmt.Errorf("请指定起始 Slave ID")
		}
		typ := b.value("type")
		if typ == "" {
			typ = "tcp"
		}
		for i := 0; i < count; i++ {
			item, _ := parseNode("name: \"\"\ntype: " + q(typ) + "\n")
			if err := apply(item.Content[0], i); err != nil {
				return err
			}
			if b.value("name_prefix") == "" {
				putText(item, "name", fmt.Sprintf("downstream-%d", i+1))
			}
			text, _ := nodeText(item.Content[0])
			_, err := appendNode(root, fmt.Sprintf("gateways.%d.downstreams", gi), text)
			if err != nil {
				return err
			}
			b.summary = append(b.summary, "新增下游 "+fieldValue(item, "name").Value+"（网关 "+g.Name+"）")
		}
	case "copy":
		for i, n := range nodes {
			source := fieldValue(root, n.path)
			text, err := nodeText(source)
			if err != nil {
				return err
			}
			copyRoot, err := parseNode(text)
			if err != nil {
				return err
			}
			copy := copyRoot.Content[0]
			if err := apply(copy, i); err != nil {
				return err
			}
			names := []string{}
			prefix := n.title() + "-copy"
			if b.value("name_prefix") != "" {
				prefix = b.value("name_prefix")
			}
			parent := n.path[:strings.LastIndex(n.path, ".")]
			switch n.kind {
			case "模拟模型":
				for _, s := range e.draft.Simulations {
					names = append(names, s.Name)
				}
				putText(copyRoot, "persistence.path", "")
			case "网关":
				for _, g := range e.draft.Gateways {
					names = append(names, g.Name)
				}
				ups := yamlAt(copy, []string{"upstreams"})
				if ups != nil {
					for _, u := range ups.Content {
						typ := yamlAt(u, []string{"type"})
						if typ != nil && typ.Value == "rtu" {
							yamlPut(u, []string{"serial", "device"}, scalar(""))
						} else {
							yamlPut(u, []string{"tcp", "address"}, scalar(""))
						}
					}
				}
			case "下游":
				for _, d := range e.draft.Gateways[n.gw].Downstreams {
					names = append(names, d.Name)
				}
				if b.value("slave_start") == "" && b.value("slave_ids") == "" {
					currentText, _ := yaml.Marshal(root)
					currentConfig, parseErr := config.ParseDraft(currentText)
					if parseErr != nil {
						return parseErr
					}
					next := freeSlaveID(currentConfig.Gateways[n.gw])
					if next == 0 {
						return fmt.Errorf("网关中没有空闲 Slave ID")
					}
					yamlPut(copy, []string{"slave_ids"}, scalar(strconv.Itoa(next)))
				}
			case "上游":
				typ := yamlAt(copy, []string{"type"})
				if typ != nil && typ.Value == "rtu" {
					yamlPut(copy, []string{"serial", "device"}, scalar(""))
				} else {
					yamlPut(copy, []string{"tcp", "address"}, scalar(""))
				}
			default:
				return fmt.Errorf("该对象不能复制")
			}
			seq := fieldValue(root, parent)
			if seq != nil {
				for _, item := range seq.Content {
					if name := yamlAt(item, []string{"name"}); name != nil {
						names = append(names, name.Value)
					}
				}
			}
			if n.kind != "上游" {
				yamlPut(copy, []string{"name"}, scalar(uniqueName(prefix, names)))
			}
			if b.independent.Value && (n.kind == "网关" || n.kind == "下游") {
				sims := map[string]string{}
				var remap func(*yaml.Node) error
				remap = func(item *yaml.Node) error {
					if item.Kind == yaml.MappingNode {
						ref := yamlAt(item, []string{"simulation", "ref"})
						if ref != nil {
							old := ref.Value
							name := sims[old]
							if name == "" {
								all := []string{}
								seq := fieldValue(root, "simulations")
								if seq != nil {
									for _, s := range seq.Content {
										if x := yamlAt(s, []string{"name"}); x != nil {
											all = append(all, x.Value)
										}
									}
								}
								name = uniqueName(old+"-copy", all)
								_, err := appendNode(root, "simulations", "name: "+q(name)+"\npersistence: {type: memory}\n")
								if err != nil {
									return err
								}
								sims[old] = name
							}
							ref.Value = name
						}
					}
					for _, child := range item.Content {
						if err := remap(child); err != nil {
							return err
						}
					}
					return nil
				}
				if err := remap(copy); err != nil {
					return err
				}
			}
			text, _ = nodeText(copy)
			_, err = appendNode(root, parent, text)
			if err != nil {
				return err
			}
			b.summary = append(b.summary, "复制 "+n.title()+" → "+fieldValue(copy, "name").Value)
		}
	case "move":
		target, err := strconv.Atoi(b.value("gateway"))
		if err != nil || target < 1 || target > len(e.draft.Gateways) {
			return fmt.Errorf("请选择有效的目标网关序号")
		}
		target--
		paths := []string{}
		copies := map[string]string{}
		identities := map[string]string{}
		for i, n := range nodes {
			if n.kind != "下游" {
				return fmt.Errorf("只能移动下游")
			}
			if n.gw == target {
				return fmt.Errorf("目标网关与源网关相同")
			}
			copy := fieldValue(root, n.path)
			if err := apply(copy, i); err != nil {
				return err
			}
			text, _ := nodeText(copy)
			copies[n.path] = text
			identities[n.path] = n.id
			paths = append(paths, n.path)
		}
		sortedPaths(paths)
		for _, path := range paths {
			if err := removeNode(root, path, ids); err != nil {
				return err
			}
		}
		for _, n := range nodes {
			dest, err := appendNode(root, fmt.Sprintf("gateways.%d.downstreams", target), copies[n.path])
			if err != nil {
				return err
			}
			ids[dest] = identities[n.path]
			b.summary = append(b.summary, fmt.Sprintf("移动 %s → %s", n.title(), e.draft.Gateways[target].Name))
		}
	case "delete":
		paths := []string{}
		selected := map[string]bool{}
		for _, n := range nodes {
			selected[n.path] = true
		}
		covered := func(path string) bool {
			for p := range selected {
				if path == p || strings.HasPrefix(path, p+".") {
					return true
				}
			}
			return false
		}
		// Resolve references against the entire removal set before shifting indices.
		for _, n := range nodes {
			if n.kind == "模拟模型" {
				for _, ref := range e.references(n.title()) {
					if covered(ref) {
						continue
					}
					if b.cascade.Value {
						selected[ref] = true
						continue
					}
					replacement := b.value("simulation.ref")
					valid := false
					for si, s := range e.draft.Simulations {
						if s.Name == replacement && !selected[fmt.Sprintf("simulations.%d", si)] {
							valid = true
						}
					}
					if !valid {
						return fmt.Errorf("模型 %s 仍被 %s 引用，请选择替代模型或明确级联删除", n.title(), e.referenceLabel(ref))
					}
					putText(root, ref+".simulation.ref", replacement)
					b.summary = append(b.summary, "迁移 "+e.referenceLabel(ref)+" → "+replacement)
				}
			}
		}
		for path := range selected {
			skip := false
			for p := range selected {
				if p != path && strings.HasPrefix(path, p+".") {
					skip = true
				}
			}
			if !skip {
				paths = append(paths, path)
			}
		}
		sortedPaths(paths)
		for _, path := range paths {
			if err := removeNode(root, path, ids); err != nil {
				return err
			}
			b.summary = append(b.summary, "删除 "+e.node(path).title())
		}
	default:
		return fmt.Errorf("未知批量操作")
	}
	bytes, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	cfg, err := config.ParseDraft(bytes)
	if err != nil {
		return err
	}
	b.problems = editorProblems(cfg)
	if b.kind == "move" || b.kind == "edit" || b.kind == "add" || b.kind == "delete" {
		for _, p := range cfg.Problems() {
			return fmt.Errorf("操作存在冲突：%s", p.Message)
		}
	}
	b.preview = &structureSnapshot{text: string(bytes), selected: e.sel, ids: ids, next: e.nextID}
	b.revision = config.Revision([]byte(e.recoveryContent()))
	b.requestRevision = b.fingerprint()
	return nil
}
func freeSlaveID(g config.GatewayConfig) int {
	used := map[byte]bool{}
	for _, d := range g.Downstreams {
		ids, _ := routing.ParseSlaveIDs(d.SlaveIDs)
		for _, id := range ids {
			used[id] = true
		}
	}
	for id := 1; id <= 247; id++ {
		if !used[byte(id)] {
			return id
		}
	}
	return 0
}
func (e *configEditor) confirmBulk() error {
	b := e.wb.bulk
	if b == nil || b.preview == nil {
		return fmt.Errorf("请先预览操作")
	}
	if b.revision != config.Revision([]byte(e.recoveryContent())) || b.requestRevision != b.fingerprint() {
		return fmt.Errorf("草稿已经变化，请重新预览")
	}
	before := e.snapshot()
	if err := e.restore(b.preview); err != nil {
		return err
	}
	e.recordEdit(before, "批量"+b.kind)
	e.wb.bulk = nil
	e.wb.selected = map[string]*widget.Bool{}
	return nil
}
func (e *configEditor) bulkPane(gtx C) D {
	b := e.wb.bulk
	labels := map[string]string{"copy": "复制配置", "edit": "批量修改", "move": "移动下游", "delete": "删除影响审阅", "add": "批量新增下游"}
	items := []layout.Widget{e.th.bold(labels[b.kind], titleSize, colInk).Layout}
	for i, g := range e.draft.Gateways {
		if b.kind == "add" || b.kind == "move" {
			items = append(items, e.th.label(fmt.Sprintf("网关 %d：%s", i+1, g.Name), smallSize, colBody).Layout)
		}
	}
	fields := []struct{ key, label, value string }{}
	if b.kind == "move" || b.kind == "add" {
		fields = append(fields, struct{ key, label, value string }{"gateway", "目标网关序号", "1"})
	}
	if b.kind == "add" {
		fields = append(fields, struct{ key, label, value string }{"count", "新增数量", "1"})
	}
	if b.kind == "add" || b.kind == "edit" || b.kind == "copy" || b.kind == "move" {
		for _, f := range []struct{ key, label, value string }{{"name_prefix", "名称前缀（留空保留）", ""}, {"type", "协议类型（留空保留）", ""}, {"tcp.address", "TCP 地址（留空保留）", ""}, {"serial.device", "串口（留空保留）", ""}, {"serial.baud_rate", "波特率（留空保留）", ""}, {"slave_start", "连续 Slave ID 起点（可选）", ""}, {"simulation.ref", "绑定模型（留空保留）", ""}} {
			fields = append(fields, f)
		}
	}
	if b.kind == "delete" {
		fields = append(fields, struct{ key, label, value string }{"simulation.ref", "剩余引用迁移到模型（可选）", ""})
	}
	containersOnly := len(b.ids) > 0
	for _, id := range b.ids {
		n := e.nodeByID(id)
		if n == nil || n.kind == "下游" || n.kind == "上游" {
			containersOnly = false
		}
	}
	for _, f := range fields {
		if containersOnly && b.kind == "copy" && f.key != "name_prefix" {
			continue
		}
		items = append(items, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(e.th.label(f.label, smallSize, colMuted).Layout), layout.Rigid(func(gtx C) D {
				if f.key == "simulation.ref" {
					return e.wbButton(gtx, "picker|bulk", orDash(b.value(f.key))+" ...", btnDefault)
				}
				return e.searchField(gtx, b.field(f.key, f.value), "")
			}))
		})
	}
	if b.kind == "copy" {
		items = append(items, func(gtx C) D {
			return material.CheckBox(e.th.Theme, &b.independent, "创建独立模拟模型（默认共享；新模型使用 memory）").Layout(gtx)
		})
	}
	if b.kind == "delete" {
		items = append(items, func(gtx C) D {
			return material.CheckBox(e.th.Theme, &b.cascade, "明确连同未被选择的引用下游删除").Layout(gtx)
		})
	}
	for _, id := range b.ids {
		if n := e.nodeByID(id); n != nil {
			items = append(items, e.th.label(n.kind+" · "+n.title()+" · "+e.describe(n), smallSize, colBody).Layout)
		}
	}

	for _, s := range b.summary {
		items = append(items, e.th.label(s, smallSize, colBody).Layout)
	}
	for _, p := range b.problems {
		items = append(items, e.th.label("待补全："+e.bulkProblemLabel(b, p), smallSize, colWarn).Layout)
	}
	if e.structureErr != "" {
		items = append(items, e.th.label(e.structureErr, textSize, colErr).Layout)
	}
	return layout.Inset{Left: 24, Right: 24, Top: 12, Bottom: 12}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return row(gtx, 36, layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "preview-bulk", "预览影响", btnDefault) }), gap(6), layout.Rigid(func(gtx C) D {
					draw := func(gtx C) D { return e.wbButton(gtx, "confirm-bulk", "确认写入草稿", btnPrimary) }
					if b.preview == nil || b.requestRevision != b.fingerprint() {
						return disabled(gtx, draw)
					}
					return draw(gtx)
				}), gap(6), layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "cancel-bulk", "取消", btnDefault) }))
			}),
			layout.Flexed(1, func(gtx C) D {
				return material.List(e.th.Theme, &b.list).Layout(gtx, len(items), func(gtx C, i int) D { return layout.Inset{Bottom: 8}.Layout(gtx, items[i]) })
			}))
	})
}

func (b *bulkOperation) fingerprint() string {
	keys := []string{}
	for k := range b.fields {
		if b.value(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var text strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&text, "%s=%s\n", k, b.value(k))
	}
	fmt.Fprintf(&text, "%v/%v", b.independent.Value, b.cascade.Value)
	return config.Revision([]byte(text.String()))
}
func (e *configEditor) startWizard() error {
	if err := e.mutateStructure("add-gateway", "", nil); err != nil {
		return err
	}
	e.wb.wizard = &gatewayWizard{id: e.node(e.sel).id}
	e.wb.notice = "新建网关 1/3：设置名称，完成后点击下一步。"
	return nil
}
func (e *configEditor) wizardNext() error {
	w := e.wb.wizard
	if w == nil {
		return nil
	}
	if err := e.finishEdits(); err != nil {
		return err
	}
	g := e.nodeByID(w.id)
	if g == nil {
		return fmt.Errorf("引导网关已被删除")
	}
	switch w.step {
	case 0:
		if strings.TrimSpace(g.title()) == "" {
			return fmt.Errorf("请填写网关名称")
		}
		if err := e.mutateStructure("add-upstream", g.path, nil); err != nil {
			return err
		}
		w.step = 1
		e.wb.notice = "新建网关 2/3：设置上游协议和监听地址。"
	case 1:
		cfg := e.draft.Gateways[g.gw]
		for _, u := range cfg.Upstreams {
			if msg := linkProblem(u.Type, u.Tcp, u.Serial); msg != "" {
				return fmt.Errorf("请补全上游：%s", msg)
			}
		}
		if err := e.mutateStructure("add-downstream", g.path, nil); err != nil {
			return err
		}
		w.step = 2
		e.wb.notice = "新建网关 3/3：设置下游、Slave ID 和模型引用，再检查配置。"
	case 2:
		for _, p := range editorProblems(e.draft) {
			if strings.HasPrefix(problemPath(p), g.path+".") {
				return fmt.Errorf("请修正配置：%s", p.Message)
			}
		}
		e.navigate(g.path)
		e.wb.wizard = nil
		e.wb.notice = "网关配置已完成，可查看变更后保存。"
		e.showDiff = true
	}
	return nil
}

func (e *configEditor) bulkProblemLabel(b *bulkOperation, p config.Problem) string {
	path := problemPath(p)
	if b.preview != nil {
		root, err := parseNode(b.preview.text)
		if err == nil {
			parts := strings.Split(path, ".")
			for len(parts) > 0 {
				item := fieldValue(root, strings.Join(parts, "."))
				if item != nil {
					name := fieldValue(item, "name")
					if name != nil && name.Value != "" {
						return name.Value + "：" + p.Message
					}
				}
				parts = parts[:len(parts)-1]
			}
		}
	}
	return p.Message
}
