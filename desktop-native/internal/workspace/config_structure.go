package workspace

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/ffutop/modbus-gateway/internal/config"
	"gopkg.in/yaml.v3"
	"net"
	"sort"
	"strconv"
	"strings"
)

type structureSnapshot struct {
	buffers        map[string]string
	raw            bool
	text, selected string
	ids            map[string]string
	next           int
}

func copyIDs(src map[string]string) map[string]string {
	dst := map[string]string{}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
func (e *configEditor) snapshot() *structureSnapshot {
	text := e.visualYAML()
	if e.raw {
		text = e.rawEd.Text()
	}
	buffers := map[string]string{}
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if ed := e.eds[s.path]; ed != nil && ed.Text() != s.get() {
				buffers[s.path] = ed.Text()
			}
		}
	}
	return &structureSnapshot{text: text, selected: e.sel, ids: copyIDs(e.ids), next: e.nextID, buffers: buffers, raw: e.raw}
}
func (e *configEditor) restore(s *structureSnapshot) error {
	cfg, err := config.ParseDraft([]byte(s.text))
	if err != nil {
		e.raw = true
		e.rawErr = err.Error()
		e.rawEd.SetText(s.text)
		e.sel = s.selected
		return nil
	}
	e.draft, e.rawSynced, e.sel, e.ids, e.nextID = cfg, s.text, s.selected, copyIDs(s.ids), s.next
	e.resetControls()
	e.rebuild()
	e.rawEd.SetText(s.text)
	e.raw = s.raw
	for path, value := range s.buffers {
		if ed := e.eds[path]; ed != nil {
			ed.SetText(value)
		}
	}
	return nil
}
func (e *configEditor) resetControls() {
	e.inputText = map[string]string{}
	e.eds = map[string]*widget.Editor{}
	e.opts = map[string]clicks[string]{}
	e.treeBtn = clicks[string]{}
	e.mapAdd, e.mapRemove = clicks[string]{}, clicks[string]{}
	e.structure = clicks[string]{}
	e.deletePath = ""
	e.structureErr = ""
	e.structureNotice = ""
}
func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }
func parseNode(text string) (*yaml.Node, error) {
	var n yaml.Node
	err := yaml.Unmarshal([]byte(text), &n)
	return &n, err
}
func putText(n *yaml.Node, path, value string) { yamlPut(n, strings.Split(path, "."), scalar(value)) }
func appendNode(root *yaml.Node, path, text string) (string, error) {
	n, err := parseNode(text)
	if err != nil {
		return "", err
	}
	parts := strings.Split(path, ".")
	seq := yamlAt(root, parts)
	if seq == nil || seq.Kind != yaml.SequenceNode {
		// yamlPut copies the node into the tree; append to that copy.
		yamlPut(root, parts, &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
		seq = yamlAt(root, parts)
	}
	index := len(seq.Content)
	seq.Content = append(seq.Content, n.Content[0])
	return fmt.Sprintf("%s.%d", path, index), nil
}

// Shift addresses while keeping surviving entities' identities unchanged.
func removeNode(root *yaml.Node, path string, ids map[string]string) error {
	parts := strings.Split(path, ".")
	i, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return err
	}
	prefix := strings.Join(parts[:len(parts)-1], ".")
	seq := yamlAt(root, parts[:len(parts)-1])
	if seq == nil || i < 0 || i >= len(seq.Content) {
		return fmt.Errorf("配置节点已不存在")
	}
	seq.Content = append(seq.Content[:i], seq.Content[i+1:]...)
	updated := map[string]string{}
	for p, id := range ids {
		if !strings.HasPrefix(p, prefix+".") {
			updated[p] = id
			continue
		}
		tail := strings.SplitN(strings.TrimPrefix(p, prefix+"."), ".", 2)
		idx, _ := strconv.Atoi(tail[0])
		if idx == i {
			continue
		}
		if idx > i {
			idx--
		}
		target := fmt.Sprintf("%s.%d", prefix, idx)
		if len(tail) == 2 {
			target += "." + tail[1]
		}
		updated[target] = id
	}
	for p := range ids {
		delete(ids, p)
	}
	for p, id := range updated {
		ids[p] = id
	}
	return nil
}
func uniqueName(prefix string, names []string) string {
	used := map[string]bool{}
	for _, n := range names {
		used[n] = true
	}
	for i := 1; ; i++ {
		n := fmt.Sprintf("%s-%d", prefix, i)
		if !used[n] {
			return n
		}
	}
}
func (e *configEditor) references(name string) []string {
	if e.refsCache == nil || e.refsVersion != e.version {
		e.refsCache = map[string][]string{}
		for gi, g := range e.draft.Gateways {
			for di, d := range g.Downstreams {
				if d.Type == "local" || d.Type == "injector" {
					e.refsCache[d.SimulationRef] = append(e.refsCache[d.SimulationRef], fmt.Sprintf("gateways.%d.downstreams.%d", gi, di))
				}
			}
		}
		e.refsVersion = e.version
	}
	return e.refsCache[name]
}

// Work on detached YAML and identity maps; commit only after parsing and migration checks.
func (e *configEditor) mutateStructure(action, path string, replacements map[string]string) error {

	if action != "rename" {
		if err := e.finishEdits(); err != nil {
			return err
		}
	}
	before := e.snapshot()
	root, err := parseNode(before.text)
	if err != nil {
		return err
	}
	ids := copyIDs(e.ids)
	selected := path
	switch action {
	case "add-simulation", "new-model":
		names := []string{}
		for _, s := range e.draft.Simulations {
			names = append(names, s.Name)
		}
		name := uniqueName("simulation", names)
		if replacements["name"] != "" {
			name = replacements["name"]
		}
		persistence := "memory"
		if replacements["persistence"] != "" {
			persistence = replacements["persistence"]
		}
		selected, err = appendNode(root, "simulations", "name: "+q(name)+"\npersistence: {type: "+q(persistence)+", path: "+q(replacements["path"])+"}\n")
		if action == "new-model" {
			putText(root, path+".simulation.ref", name)
			selected = path
		}
	case "add-gateway":
		names := []string{}
		for _, g := range e.draft.Gateways {
			names = append(names, g.Name)
		}
		name := uniqueName("gateway", names)
		if replacements["name"] != "" {
			name = replacements["name"]
		}
		selected, err = appendNode(root, "gateways", "name: "+q(name)+"\nupstreams: []\ndownstreams: []\n")
	case "add-upstream":
		selected, err = appendNode(root, path+".upstreams", "type: tcp\ntcp: {address: \"\"}\n")
	case "add-downstream":
		gi, _ := strconv.Atoi(strings.Split(path, ".")[1])
		g := e.draft.Gateways[gi]
		if len(g.Downstreams) == 1 && g.Downstreams[0].SlaveIDs == "" {
			return fmt.Errorf("原下游使用默认路由，请先为它指定从站 ID，再添加下游")
		}
		names := []string{}
		for _, d := range g.Downstreams {
			names = append(names, d.Name)
		}
		selected, err = appendNode(root, path+".downstreams", "name: "+q(uniqueName("downstream", names))+"\ntype: tcp\nslave_ids: \"\"\ntcp: {address: \"\"}\n")
	case "rename":
		for _, node := range e.nodes {
			if node.kind == "模拟模型" && node.path != path {
				if ed := e.eds[node.path+".name"]; ed != nil && ed.Text() != node.title() {
					return fmt.Errorf("请先应用另一个模型的改名或撤销输入")
				}
			}
		}
		n := e.node(path)
		if n == nil || n.kind != "模拟模型" {
			return fmt.Errorf("请选择模拟模型")
		}
		name := strings.TrimSpace(replacements["name"])
		if name == "" {
			return fmt.Errorf("名称不能为空")
		}
		for _, s := range e.draft.Simulations {
			if s.Name == name && s.Name != n.title() {
				return fmt.Errorf("模拟模型名称已存在")
			}
		}
		for _, ref := range e.references(n.title()) {
			putText(root, ref+".simulation.ref", name)
		}
		putText(root, path+".name", name)
	case "delete", "cascade":
		n := e.node(path)
		if n == nil {
			return fmt.Errorf("配置节点已不存在")
		}
		if n.kind == "模拟模型" {
			refs := e.references(n.title())
			if action == "cascade" {
				sort.Slice(refs, func(i, j int) bool {
					a, b := strings.Split(refs[i], "."), strings.Split(refs[j], ".")
					ag, _ := strconv.Atoi(a[1])
					bg, _ := strconv.Atoi(b[1])
					ad, _ := strconv.Atoi(a[3])
					bd, _ := strconv.Atoi(b[3])
					return ag > bg || ag == bg && ad > bd
				})
				for _, ref := range refs {
					if err = removeNode(root, ref, ids); err != nil {
						return err
					}
				}
			} else {
				for _, ref := range refs {
					target := replacements[ref]
					found := false
					for _, s := range e.draft.Simulations {
						if s.Name == target && s.Name != n.title() {
							found = true
						}
					}
					if !found {
						return fmt.Errorf("请为每条引用选择替代模型，或明确连同引用下游删除")
					}
					putText(root, ref+".simulation.ref", target)
				}
				bytes, _ := yaml.Marshal(root)
				cfg, parseErr := config.ParseDraft(bytes)
				if parseErr != nil {
					return parseErr
				}
				for _, p := range cfg.Problems() {
					if len(refs) > 0 && (strings.Contains(p.Message, "mapping") || strings.Contains(p.Message, "target range") || strings.Contains(p.Message, "simulation ref")) {
						return fmt.Errorf("迁移失败：%s", p.Message)
					}
				}
			}
		}
		err = removeNode(root, path, ids)
		parts := strings.Split(path, ".")
		selected = "global"
		if len(parts) == 2 {
			group := yamlAt(root, parts[:1])
			index, _ := strconv.Atoi(parts[1])
			if group != nil && len(group.Content) > 0 {
				if index >= len(group.Content) {
					index = len(group.Content) - 1
				}
				selected = fmt.Sprintf("%s.%d", parts[0], index)
			}
		}
		if len(parts) > 2 {
			selected = strings.Join(parts[:2], ".")
		}
	default:
		return fmt.Errorf("未知编辑操作")
	}
	if err != nil {
		return err
	}
	bytes, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	cfg, err := config.ParseDraft(bytes)
	if err != nil {
		return err
	}
	e.draft, e.rawSynced, e.ids, e.sel = cfg, string(bytes), ids, selected
	e.resetControls()
	e.undoState = before
	e.rebuild()
	e.undoAfter = e.snapshot()
	e.undoFields = map[string]string{}
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if s.visible() {
				e.undoFields[e.fieldKey(n, s)] = s.get()
			}
		}
	}
	e.rawEd.SetText(string(bytes))
	e.recordEdit(before, actionLabel(action))
	if action == "delete" || action == "cascade" {
		names := []string{}
		for _, sim := range e.draft.Simulations {
			if len(e.references(sim.Name)) == 0 {
				names = append(names, sim.Name)
			}
		}
		if len(names) > 0 {
			e.structureNotice = "无引用模型已保留，可按需删除：" + strings.Join(names, "、")
		}
	}
	return nil
}
func (e *configEditor) updateStructure(gtx C) bool {
	if e.undoBtn.Clicked(gtx) && e.undoState != nil {
		if err := e.undoHistory(); err != nil {
			e.structureErr = err.Error()
		}
		return true
	}
	if e.renameBtn.Clicked(gtx) {
		if err := e.finishEdits(); err != nil {
			e.structureErr = err.Error()
		}
		return true

	}
	for command, b := range e.structure {
		if b.Clicked(gtx) {
			parts := strings.SplitN(command, "|", 2)
			action, path := parts[0], ""
			if len(parts) == 2 {
				path = parts[1]
			}
			if e.inputBlocked && action != "cancel-delete" && action != "replacement" && action != "replace-all" && action != "delete" && action != "cascade" {
				continue
			}
			switch action {
			case "add-gateway", "add-simulation", "new-model":
				e.openCreation(action, path)
			case "request-delete":
				e.deletePath = path
				e.dialogList.Position = layout.Position{}
				e.replacements = map[string]string{}
				e.structureErr = ""
			case "cancel-delete":
				e.deletePath = ""
				e.wb.picker = ""
				e.structureErr = ""
			case "replace-all":
				for _, ref := range e.references(e.node(e.deletePath).title()) {
					e.replacements[ref] = path
				}
			case "select":
				e.navigate(path)
			case "replacement":
				choice := strings.SplitN(path, "|", 2)
				e.replacements[choice[0]] = choice[1]
			default:
				if err := e.mutateStructure(action, path, e.replacements); err != nil {
					e.structureErr = err.Error()
				}
			}
			return true
		}
	}
	return false
}
func (e *configEditor) structureActions(gtx C, n *cfgNode) D {
	if e.structure == nil {
		e.structure = clicks[string]{}
	}
	var items []layout.FlexChild
	button := func(action, path, label string) {
		kind := btnDefault
		switch action {
		case "select":
			kind = btnLink
		case "request-delete", "cascade":
			kind = btnDanger
		}
		items = append(items, layout.Rigid(func(gtx C) D { return e.th.button(gtx, e.structure.get(action+"|"+path), label, kind) }))
	}
	if e.deletePath == "" {
		if n.kind == "网关" {
			button("add-upstream", n.path, "+ 添加上游")
			button("add-downstream", n.path, "+ 添加下游")
		}
		if n.kind == "模拟模型" {

			refs := e.references(n.title())
			items = append(items, layout.Rigid(e.th.label(fmt.Sprintf("被 %d 个下游引用", len(refs)), smallSize, colMuted).Layout))
			for _, ref := range refs {
				button("select", ref, e.referenceLabel(ref))
			}
		}
		if d := e.downstream(n.path); d != nil && (d.Type == "local" || d.Type == "injector") {
			button("new-model", n.path, "+ 新建并引用模拟模型")
		}
		if n.kind != "常规" {
			button("request-delete", n.path, "删除"+n.kind)
		}

	}

	if e.deletePath != "" {
		target := e.node(e.deletePath)
		if target != nil {
			warning := "确认删除 " + target.kind + " " + target.title() + "？"
			if target.kind == "网关" {
				g := e.draft.Gateways[target.gw]
				warning += fmt.Sprintf("将连同 %d 个上游、%d 个下游删除；共享模拟模型保留。", len(g.Upstreams), len(g.Downstreams))
			}
			if target.kind == "模拟模型" {
				warning += "持久化数据保留。"
			}
			items = append(items, layout.Rigid(e.th.label(warning, textSize, colWarn).Layout))
			if target.kind == "网关" {
				for _, child := range e.nodes {
					if child.depth > 0 && child.gw == target.gw {
						items = append(items, layout.Rigid(e.th.label(child.kind+" · "+child.title(), smallSize, colBody).Layout))
					}
				}
			}
			if target.kind == "模拟模型" && len(e.references(target.title())) > 0 {
				if len(e.references(target.title())) > 0 && len(e.draft.Simulations) < 2 {
					items = append(items, layout.Rigid(e.th.label("没有替代模型；可取消后新增模型，再迁移引用。", smallSize, colWarn).Layout))
				}
				items = append(items, layout.Rigid(func(gtx C) D { return e.wbButton(gtx, "picker|replace-all", "统一选择替代模型", btnDefault) }))
				for _, ref := range e.references(target.title()) {
					items = append(items, layout.Rigid(func(gtx C) D {
						return row(gtx, 34, layout.Flexed(1, e.th.label(e.referenceLabel(ref), smallSize, colBody).Layout), layout.Rigid(func(gtx C) D {
							return e.wbButton(gtx, "picker|replacement:"+ref, orDash(e.replacements[ref])+" ...", btnDefault)
						}))
					}))
				}
				if len(e.references(target.title())) > 0 {
					button("cascade", target.path, "确认连同上述引用下游删除（网关可能变空）")
				}
			}
			canDelete := true
			label := "确认删除"
			if target.kind == "模拟模型" && len(e.references(target.title())) > 0 {
				label = "确认迁移引用并删除"
				for _, ref := range e.references(target.title()) {
					if e.replacements[ref] == "" {
						canDelete = false
					}
				}
			}
			items = append(items, layout.Rigid(func(gtx C) D {
				draw := func(gtx C) D { return e.th.button(gtx, e.structure.get("delete|"+target.path), label, btnDanger) }
				if !canDelete {
					return disabled(gtx, draw)
				}
				return draw(gtx)
			}))

		}
	}
	if e.structureNotice != "" {
		items = append(items, layout.Rigid(e.th.label(e.structureNotice, smallSize, colMuted).Layout))
	}
	if e.structureErr != "" {
		items = append(items, layout.Rigid(e.th.label(e.structureErr, textSize, colErr).Layout))
	}
	spaced := make([]layout.FlexChild, 0, 2*len(items))
	for _, it := range items {
		spaced = append(spaced, it, vgap(6))
	}
	return layout.Inset{Left: 24, Right: 24, Top: 6, Bottom: 12}.Layout(gtx, func(gtx C) D { return layout.Flex{Axis: layout.Vertical}.Layout(gtx, spaced...) })
}
func (e *configEditor) referenceLabel(path string) string {
	n := e.node(path)
	d := e.downstream(path)
	if n == nil || d == nil {
		return path
	}
	return fmt.Sprintf("%s / %s · %s · ID %s", e.draft.Gateways[n.gw].Name, d.Name, d.Type, d.SlaveIDs)
}

// Desktop completeness checks deliberately leave the CLI schema unchanged.
func editorProblems(c *config.Config) []config.Problem {
	ps := c.Problems()
	if c.Version != 1 {
		return ps
	}
	add := func(path []any, msg string) { ps = append(ps, config.Problem{Path: path, Message: msg}) }
	if len(c.Gateways) == 0 {
		add([]any{"gateways"}, "至少需要一个网关")
	}
	listeners := []config.UpstreamConfig{}
	for gi, g := range c.Gateways {
		if len(g.Upstreams) == 0 {
			add([]any{"gateways", gi, "upstreams"}, "网关至少需要一个上游")
		}
		if len(g.Downstreams) == 0 {
			add([]any{"gateways", gi, "downstreams"}, "网关至少需要一个下游")
		}

		for ui, u := range g.Upstreams {
			path := []any{"gateways", gi, "upstreams", ui}
			if msg := linkProblem(u.Type, u.Tcp, u.Serial); msg != "" {
				add(path, msg)
			}
			if u.Type == "tcp" || u.Type == "rtu-over-tcp" {
				host, port, err := net.SplitHostPort(u.Tcp.Address)
				if err == nil && port != "0" {
					for _, other := range listeners {
						oh, op, oe := net.SplitHostPort(other.Tcp.Address)
						if oe == nil && port == op && host != oh && (wildcardHost(host) || wildcardHost(oh)) {
							add(path, "上游监听端口与通配地址冲突")
						}
					}
				}
			}
			listeners = append(listeners, u)
		}
		for di, d := range g.Downstreams {
			if d.Type != "local" && d.Type != "injector" {
				if msg := linkProblem(d.Type, d.Tcp, d.Serial); msg != "" {
					add([]any{"gateways", gi, "downstreams", di}, msg)
				}
			}
			if d.SlaveIDs == "" && (len(g.Downstreams) > 1 || d.Type == "local" || d.Type == "injector") {
				add([]any{"gateways", gi, "downstreams", di, "slave_ids"}, "请指定从站 ID，避免下游不可达")
			}
		}
	}

	for si, s := range c.Simulations {
		if strings.TrimSpace(s.Name) == "" {
			add([]any{"simulations", si, "name"}, "模型名称不能为空")
		}
		switch s.Persistence.Type {
		case "", "memory":
		case "file", "mmap", "sql":
			if strings.TrimSpace(s.Persistence.Path) == "" {
				add([]any{"simulations", si, "persistence", "path"}, "持久化路径不能为空")
			}
		default:
			add([]any{"simulations", si, "persistence", "type"}, "未知持久化类型")
		}
	}
	return ps
}
func wildcardHost(host string) bool { return host == "" || host == "0.0.0.0" || host == "::" }
func linkProblem(typ string, tcp config.TcpConfig, serial config.SerialConfig) string {
	switch typ {
	case "tcp", "rtu-over-tcp":
		_, port, err := net.SplitHostPort(tcp.Address)
		if err != nil {
			return "地址应为主机:端口"
		}
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return "端口应在 0–65535 范围内"
		}
	case "rtu":
		if strings.TrimSpace(serial.Device) == "" {
			return "串口设备不能为空"
		}
		if serial.BaudRate <= 0 {
			return "波特率必须大于 0"
		}
	default:
		return "未知链路类型"
	}
	return ""
}

func (e *configEditor) pendingRename() bool {
	for _, n := range e.nodes {
		if n.kind == "模拟模型" {
			if ed := e.eds[n.path+".name"]; ed != nil && ed.Text() != n.title() {
				return true
			}
		}
	}
	return false
}

// YAML reparsing matches named entities without ever reusing one identity twice; positional
// upstreams retain identity only when their full configuration is unchanged.
func (e *configEditor) reconcileRawIDs() {
	old, err := config.ParseDraft([]byte(e.rawSynced))
	if err != nil {
		e.ids = map[string]string{}
		return
	}
	ids := map[string]string{"global": e.ids["global"]}
	usedIDs := map[string]bool{e.ids["global"]: true}
	retain := func(path, previous string) {
		id := e.ids[previous]
		if id != "" && !usedIDs[id] && ids[path] == "" {
			ids[path] = id
			usedIDs[id] = true
		}
	}
	for si, s := range e.draft.Simulations {
		for oi, o := range old.Simulations {
			if s.Name == o.Name {
				retain(fmt.Sprintf("simulations.%d", si), fmt.Sprintf("simulations.%d", oi))
			}
		}
	}
	for gi, g := range e.draft.Gateways {
		for oi, o := range old.Gateways {
			if g.Name != o.Name {
				continue
			}
			base, ob := fmt.Sprintf("gateways.%d", gi), fmt.Sprintf("gateways.%d", oi)
			retain(base, ob)
			for di, d := range g.Downstreams {
				for od, x := range o.Downstreams {
					if d.Name == x.Name {
						retain(fmt.Sprintf("%s.downstreams.%d", base, di), fmt.Sprintf("%s.downstreams.%d", ob, od))
					}
				}
			}
			used := map[int]bool{}
			for ui, u := range g.Upstreams {
				for ou, x := range o.Upstreams {
					if !used[ou] && u == x {
						retain(fmt.Sprintf("%s.upstreams.%d", base, ui), fmt.Sprintf("%s.upstreams.%d", ob, ou))
						used[ou] = true
						break
					}
				}
			}
		}
	}
	e.ids = ids
}
func (e *configEditor) transportDefaults(path, typ string) {
	if typ != "rtu" {
		return
	}
	parts := strings.Split(path, ".")
	if len(parts) != 5 {
		return
	}
	gi, _ := strconv.Atoi(parts[1])
	idx, _ := strconv.Atoi(parts[3])
	var serial *config.SerialConfig
	if parts[2] == "upstreams" {
		serial = &e.draft.Gateways[gi].Upstreams[idx].Serial
	} else if parts[2] == "downstreams" {
		serial = &e.draft.Gateways[gi].Downstreams[idx].Serial
	}
	if serial != nil {
		if serial.BaudRate == 0 {
			serial.BaudRate = 9600
		}
		if serial.DataBits == 0 {
			serial.DataBits = 8
		}
		if serial.Parity == "" {
			serial.Parity = "N"
		}
		if serial.StopBits == 0 {
			serial.StopBits = 1
		}
	}
}

// Undo the latest structure command while retaining later edits to surviving
// entities. Edits inside an entity created by that command are removed with it.
func (e *configEditor) undoStructure() error {
	if e.undoState == nil {
		return nil
	}
	if e.pendingRename() {
		return fmt.Errorf("请先应用模型改名或撤销输入，再撤销结构操作")
	}
	before := e.undoState
	root, err := parseNode(before.text)
	if err != nil {
		return err
	}
	current, err := parseNode(e.visualYAML())
	if err != nil {
		return err
	}
	after, err := parseNode(e.undoAfter.text)
	if err != nil {
		return err
	}
	oldPaths := map[string]string{}
	afterPaths := map[string]string{}
	for path, id := range before.ids {
		oldPaths[id] = path
	}
	for path, id := range e.undoAfter.ids {
		afterPaths[id] = path
	}
	for _, n := range e.nodes {
		oldPath, exists := oldPaths[n.id]
		if !exists {
			continue
		}
		for _, s := range n.specs {
			if !s.visible() {
				continue
			}
			key := e.fieldKey(n, s)
			if old, had := e.undoFields[key]; had && old == s.get() {
				continue
			}
			value := yamlAt(current, strings.Split(s.path, "."))
			if value != nil {
				destination := oldPath + strings.TrimPrefix(s.path, n.path)
				if n.kind == "常规" {
					destination = s.path
				}
				yamlPut(root, strings.Split(destination, "."), value)
			}
		}
		if d := e.downstream(n.path); d != nil && d.Type == "injector" {
			curr := yamlAt(current, strings.Split(n.path+".simulation.mappings", "."))
			previous := yamlAt(after, strings.Split(afterPaths[n.id]+".simulation.mappings", "."))
			a, _ := yaml.Marshal(curr)
			b, _ := yaml.Marshal(previous)
			if string(a) != string(b) {
				if curr == nil {
					curr = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
				}
				yamlPut(root, strings.Split(oldPath+".simulation.mappings", "."), curr)
			}
		}
	}
	bytes, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	cfg, err := config.ParseDraft(bytes)
	if err != nil {
		return err
	}
	for _, p := range cfg.Problems() {
		if strings.Contains(p.Message, "unknown simulation ref") {
			return fmt.Errorf("撤销会留下悬空引用，请先调整相关下游引用")
		}
	}
	restored := *before
	restored.text = string(bytes)
	if err := e.restore(&restored); err != nil {
		return err
	}
	e.undoState = nil
	return nil
}
