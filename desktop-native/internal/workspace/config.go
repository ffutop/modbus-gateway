// Package workspace implements the approved desktop workspace over live gateway data.
package workspace

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/ffutop/modbus-gateway/internal/config"
)

// spec is one editable value of the draft, addressed by a path in the same
// form as config.Problem paths ("gateways.0.downstreams.1.slave_ids").
type spec struct {
	path     string
	label    string
	hint     string
	options  []string
	get      func() string
	set      func(string)
	when     func() bool
	check    func(string) string // local checks the config package leaves to the OS
	readOnly bool
}

func (s *spec) visible() bool { return s.when == nil || s.when() }

type cfgNode struct {
	path  string // prefix of its specs
	kind  string
	title func() string
	depth int
	specs []*spec
	gw    int // gateway index, -1 if none
}

type change struct {
	node     *cfgNode
	label    string
	old, new string
	removed  bool
}

type configEditor struct {
	th         *Theme
	saveFile   func(string) error
	configPath string
	startErr   error
	running    bool

	draft        *config.Config
	baseline     map[string]string // path → value at the last save
	baseNode     map[string]string // node path → title at the last save
	savedYAML    string
	runningYAML  string
	saveFailed   bool
	saveProblem  string
	issueBtn     clicks[string]
	mapAdd       clicks[string]
	mapRemove    clicks[string]
	visualLocked bool
	nodes        []*cfgNode
	sel          string // selected node path
	eds          map[string]*widget.Editor
	opts         map[string]clicks[string]
	treeBtn      clicks[string]
	addDs        clicks[int]

	raw        bool
	modes      [2]widget.Clickable
	rawEd      widget.Editor
	rawSynced  string // raw text last generated from or parsed into the draft
	rawErr     string
	discardRaw widget.Clickable

	problems []config.Problem
	version  int // bumps on every draft change
	checked  int

	revert, diff, save widget.Clickable
	showDiff           bool
	toast              string
	toastAt            time.Time

	tree, form, side, rawList widget.List
	issueScroll               widget.List
}

func newConfigEditor(th *Theme, info Info) *configEditor {
	e := &configEditor{th: th, saveFile: info.Save, configPath: info.Config.Path, startErr: info.StartErr, running: info.Running,
		eds: map[string]*widget.Editor{}, opts: map[string]clicks[string]{}, treeBtn: clicks[string]{}, addDs: clicks[int]{}}
	e.tree.Axis, e.form.Axis, e.side.Axis, e.rawList.Axis = layout.Vertical, layout.Vertical, layout.Vertical, layout.Vertical
	e.issueScroll.Axis = layout.Vertical
	e.draft, _ = config.ParseDraft([]byte(info.Content))
	if e.draft == nil {
		e.draft = &config.Config{}
	}
	e.savedYAML, e.rawSynced = info.Content, info.Content
	e.rawEd.SetText(info.Content)
	e.rebuild()
	e.commitBaseline()
	if info.Running {
		e.runningYAML = info.Content
	}
	e.issueBtn, e.mapAdd, e.mapRemove = clicks[string]{}, clicks[string]{}, clicks[string]{}
	e.checked = -1
	e.raw = info.Config.Version != 1 || info.StartErr != nil
	e.visualLocked = info.Config.Version != 1
	if _, err := config.ParseDraft([]byte(info.Content)); err != nil {
		e.rawErr = err.Error()
		e.raw = true
	}
	return e
}

// ---- YAML ----

func q(s string) string { return strconv.Quote(s) }

// toYAML writes c as a canonical v1 file. Fields that do not apply to a
// type are left out.
func toYAML(c *config.Config) string {
	var b strings.Builder
	p := func(indent int, format string, args ...any) {
		b.WriteString(strings.Repeat("  ", indent))
		fmt.Fprintf(&b, format, args...)
		b.WriteByte('\n')
	}
	p(0, "version: 1")
	p(0, "log:")
	p(1, "level: %s", c.Log.Level)
	if c.Log.File != "" {
		p(1, "file: %s", q(c.Log.File))
	}
	p(0, "ui:")
	p(1, "enabled: %t", c.UI.Enabled)
	if c.UI.Enabled {
		p(1, "listen: %s", q(c.UI.Listen))
	}
	p(0, "simulations:")
	for _, s := range c.Simulations {
		p(1, "- name: %s", q(s.Name))
		p(2, "persistence:")
		p(3, "type: %s", s.Persistence.Type)
		if s.Persistence.Type != "memory" {
			p(3, "path: %s", q(s.Persistence.Path))
		}
	}
	p(0, "gateways:")
	for _, g := range c.Gateways {
		p(1, "- name: %s", q(g.Name))
		p(2, "upstreams:")
		for _, u := range g.Upstreams {
			p(3, "- type: %s", u.Type)
			writeLink(p, 4, u.Type, u.Tcp, u.Serial)
		}
		p(2, "downstreams:")
		for _, d := range g.Downstreams {
			p(3, "- name: %s", q(d.Name))
			p(4, "type: %s", d.Type)
			p(4, "slave_ids: %s", q(d.SlaveIDs))
			switch d.Type {
			case "local", "injector":
				p(4, "simulation:")
				p(5, "ref: %s", q(d.SimulationRef))
				if d.Type == "injector" && len(d.Mappings) > 0 {
					p(5, "mappings:")
					for _, m := range d.Mappings {
						p(6, "- source: { table: %s, start_address: %d, count: %d }", m.Source.Table, m.Source.StartAddress, m.Source.Count)
						p(7, "target: { table: %s, start_address: %d }", m.Target.Table, m.Target.StartAddress)
					}
				}
			default:
				writeLink(p, 4, d.Type, d.Tcp, d.Serial)
			}
		}
	}
	return b.String()
}

func writeLink(p func(int, string, ...any), indent int, typ string, tcp config.TcpConfig, serial config.SerialConfig) {
	if typ == "rtu" {
		p(indent, "serial:")
		p(indent+1, "device: %s", q(serial.Device))
		p(indent+1, "baud_rate: %d", serial.BaudRate)
		return
	}
	p(indent, "tcp:")
	p(indent+1, "address: %s", q(tcp.Address))
}

// ---- form model ----

func checkAddr(v string) string {
	if _, port, err := net.SplitHostPort(v); err != nil || port == "" {
		return "格式应为 主机:端口，例如 0.0.0.0:502"
	}
	return ""
}

func checkName(v string) string {
	if strings.TrimSpace(v) == "" {
		return "名称不能为空"
	}
	return ""
}

func intSpec(path, label string, p *int, options []string) *spec {
	return &spec{path: path, label: label, options: options,
		get: func() string { return strconv.Itoa(*p) },
		set: func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				*p = n
			}
		}}
}

func strSpec(path, label string, p *string) *spec {
	return &spec{path: path, label: label, get: func() string { return *p }, set: func(v string) { *p = v }}
}

// rebuild derives the nodes and specs from the draft. Call it whenever the
// draft's structure changes (added downstream, re-parsed YAML).
func (e *configEditor) rebuild() {
	c := e.draft
	var nodes []*cfgNode
	var simNames []string
	for _, s := range c.Simulations {
		simNames = append(simNames, s.Name)
	}

	g := &cfgNode{path: "global", kind: "常规", title: func() string { return "全局" }, gw: -1}
	level := strSpec("log.level", "日志级别", &c.Log.Level)
	level.options = []string{"debug", "info", "warn", "error"}
	file := strSpec("log.file", "日志文件", &c.Log.File)
	file.hint = "留空或 - 表示输出到标准输出"
	uiOn := &spec{path: "ui.enabled", label: "管理 API", options: []string{"开启", "关闭"},
		get: func() string { return map[bool]string{true: "开启", false: "关闭"}[c.UI.Enabled] },
		set: func(v string) { c.UI.Enabled = v == "开启" }}
	uiListen := strSpec("ui.listen", "监听地址", &c.UI.Listen)
	uiListen.when, uiListen.check = func() bool { return c.UI.Enabled }, checkAddr
	uiListen.hint = "Web 控制台与远程管理使用"
	g.specs = []*spec{level, file, uiOn, uiListen}
	nodes = append(nodes, g)

	for i := range c.Simulations {
		s := &c.Simulations[i]
		base := fmt.Sprintf("simulations.%d", i)
		n := &cfgNode{path: base, kind: "模拟模型", title: func() string { return s.Name }, gw: -1}
		name := strSpec(base+".name", "名称", &s.Name)
		name.check = checkName
		name.hint = "改名后，引用它的下游需要同步修改"
		pt := strSpec(base+".persistence.type", "持久化", &s.Persistence.Type)
		pt.options = []string{"memory", "file", "mmap", "sql"}
		pp := strSpec(base+".persistence.path", "路径", &s.Persistence.Path)
		pp.when = func() bool { return s.Persistence.Type != "memory" }
		n.specs = []*spec{name, pt, pp}
		nodes = append(nodes, n)
	}

	for i := range c.Gateways {
		gw := &c.Gateways[i]
		base := fmt.Sprintf("gateways.%d", i)
		gn := &cfgNode{path: base, kind: "网关", title: func() string { return gw.Name }, gw: i}
		name := strSpec(base+".name", "名称", &gw.Name)
		name.check = checkName
		gn.specs = []*spec{name}
		nodes = append(nodes, gn)
		for k := range gw.Upstreams {
			u := &gw.Upstreams[k]
			ub := fmt.Sprintf("%s.upstreams.%d", base, k)
			un := &cfgNode{path: ub, kind: "上游", depth: 1, title: func() string { return "上游 · " + u.Type }, gw: i}
			ut := strSpec(ub+".type", "类型", &u.Type)
			ut.options = []string{"tcp", "rtu-over-tcp", "rtu"}
			ua := strSpec(ub+".tcp.address", "监听地址", &u.Tcp.Address)
			ua.when, ua.check = func() bool { return u.Type != "rtu" }, checkAddr
			ud := strSpec(ub+".serial.device", "串口", &u.Serial.Device)
			ud.when = func() bool { return u.Type == "rtu" }
			baud := intSpec(ub+".serial.baud_rate", "波特率", &u.Serial.BaudRate, []string{"9600", "19200", "38400", "115200"})
			baud.when = ud.when
			un.specs = []*spec{ut, ua, ud, baud}
			nodes = append(nodes, un)
		}
		for j := range gw.Downstreams {
			d := &gw.Downstreams[j]
			db := fmt.Sprintf("%s.downstreams.%d", base, j)
			dn := &cfgNode{path: db, kind: "下游", depth: 1, title: func() string { return d.Name }, gw: i}
			name := strSpec(db+".name", "名称", &d.Name)
			name.check = checkName
			dt := strSpec(db+".type", "类型", &d.Type)
			dt.options = []string{"local", "injector", "tcp", "rtu-over-tcp", "rtu"}
			ids := strSpec(db+".slave_ids", "从站 ID", &d.SlaveIDs)
			ids.hint = "例如 1、1,3 或 10-20；local / injector 只能是一个 ID"
			sockets := func() bool { return d.Type == "tcp" || d.Type == "rtu-over-tcp" }
			serial := func() bool { return d.Type == "rtu" }
			model := func() bool { return d.Type == "local" || d.Type == "injector" }
			da := strSpec(db+".tcp.address", "地址", &d.Tcp.Address)
			da.when, da.check = sockets, checkAddr
			dd := strSpec(db+".serial.device", "串口", &d.Serial.Device)
			dd.when = serial
			baud := intSpec(db+".serial.baud_rate", "波特率", &d.Serial.BaudRate, []string{"9600", "19200", "38400", "115200"})
			baud.when = serial
			ref := strSpec(db+".simulation.ref", "模拟模型", &d.SimulationRef)
			ref.options, ref.when = simNames, model
			ref.hint = "多个下游可以引用同一个模型，共享数据"
			dn.specs = []*spec{name, dt, ids, da, dd, baud, ref}
			if d.Type == "injector" {
				for mi := range d.Mappings {
					m := &d.Mappings[mi]
					mb := fmt.Sprintf("%s.simulation.mappings.%d", db, mi)
					st := strSpec(mb+".source.table", fmt.Sprintf("映射 %d · 源表", mi+1), &m.Source.Table)
					st.options = []string{"coils", "holding_registers"}
					tt := strSpec(mb+".target.table", "目标表", &m.Target.Table)
					tt.options = []string{"discrete_inputs", "input_registers"}
					dn.specs = append(dn.specs, st, uintSpec(mb+".source.start_address", "源地址", &m.Source.StartAddress), uintSpec(mb+".source.count", "数量", &m.Source.Count), tt, uintSpec(mb+".target.start_address", "目标地址", &m.Target.StartAddress))
				}
			}
			nodes = append(nodes, dn)
		}
	}
	e.nodes = nodes
	for _, n := range nodes {
		for _, s := range n.specs {
			if s.options != nil || s.readOnly {
				continue
			}
			ed := e.eds[s.path]
			if ed == nil {
				ed = &widget.Editor{SingleLine: true}
				e.eds[s.path] = ed
			}
			if ed.Text() != s.get() {
				ed.SetText(s.get())
			}
		}
	}
	if e.node(e.sel) == nil {
		e.sel = "global"
	}
	e.version++
}

func (e *configEditor) node(path string) *cfgNode {
	for _, n := range e.nodes {
		if n.path == path {
			return n
		}
	}
	return nil
}

// commitBaseline makes the draft the saved state.
func (e *configEditor) commitBaseline() {
	e.baseline, e.baseNode = map[string]string{}, map[string]string{}
	for _, n := range e.nodes {
		e.baseNode[n.path] = n.title()
		for _, s := range n.specs {
			if s.visible() {
				e.baseline[s.path] = s.get()
			}
		}
	}
}

func (e *configEditor) changes() []change {
	var out []change
	seen := map[string]bool{}
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if !s.visible() {
				continue
			}
			seen[s.path] = true
			old, ok := e.baseline[s.path]
			if !ok || old != s.get() {
				out = append(out, change{node: n, label: s.label, old: old, new: s.get()})
			}
		}
	}
	for path, old := range e.baseline {
		if !seen[path] && e.node(path[:strings.LastIndexByte(path, '.')]) == nil {
			out = append(out, change{label: path, old: old, removed: true})
		}
	}
	return out
}

func (e *configEditor) isNew(n *cfgNode) bool {
	_, ok := e.baseNode[n.path]
	return !ok
}

func problemPath(p config.Problem) string {
	parts := make([]string, len(p.Path))
	for i, x := range p.Path {
		parts[i] = fmt.Sprint(x)
	}
	return strings.Join(parts, ".")
}

// fieldErr is the message for spec s: a local check, else a config Problem
// whose path is the spec's.
func (e *configEditor) fieldErr(s *spec) string {
	if !s.visible() {
		return ""
	}
	if s.check != nil {
		value := s.get()
		if ed := e.eds[s.path]; ed != nil {
			value = ed.Text()
		}
		if msg := s.check(value); msg != "" {
			return msg
		}
	}
	for _, p := range e.problems {
		if problemPath(p) == s.path {
			return p.Message
		}
	}
	return ""
}

// nodeProblems are the Problems that belong to n but not to one of its
// visible fields.
func (e *configEditor) nodeProblems(n *cfgNode) []string {
	var out []string
	for _, p := range e.problems {
		pp := problemPath(p)
		if e.deepest(pp) != n {
			continue
		}
		attached := false
		for _, s := range n.specs {
			attached = attached || s.path == pp && s.visible()
		}
		if !attached {
			out = append(out, p.Message)
		}
	}
	return out
}

// deepest is the node with the longest path prefixing pp; problems outside
// every node belong to the global node.
func (e *configEditor) deepest(pp string) *cfgNode {
	best := e.nodes[0]
	for _, n := range e.nodes {
		if strings.HasPrefix(pp, n.path) && len(n.path) > len(best.path) {
			best = n
		}
	}
	return best
}

func (e *configEditor) errorCount() int {
	if e.raw && e.rawErr != "" {
		return 1
	}
	n := len(e.problems)
	for _, node := range e.nodes {
		for _, s := range node.specs {
			if s.check != nil && s.visible() && e.fieldErr(s) != "" {
				n++
			}
		}
	}
	return n
}

// ---- frame ----

// unsaved reports edits not yet written to the file.
func (e *configEditor) unsaved() bool {
	return len(e.changes()) > 0 || e.raw && e.rawEd.Text() != e.savedYAML
}

func (e *configEditor) Layout(gtx C) D {
	th := e.th
	e.update(gtx)
	if e.checked != e.version {
		e.problems, e.checked = e.draft.Problems(), e.version
	}
	changes := e.changes()
	if e.raw && e.rawEd.Text() != e.savedYAML && len(changes) == 0 {
		changes = append(changes, change{label: "文件文本", old: e.savedYAML, new: e.rawEd.Text()})
	}
	errs := e.errorCount()
	canSave := len(changes) > 0 && errs == 0
	if e.save.Clicked(gtx) && canSave {
		if err := e.doSave(); err != nil {
			e.saveFailed, e.saveProblem = true, err.Error()
			e.toast, e.toastAt = "保存失败，草稿和运行配置保留。", gtx.Now
		} else {
			e.saveFailed, e.saveProblem = false, ""
			e.toast, e.toastAt = "已保存。重启网关后使用新配置。", gtx.Now
		}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					if e.startErr == nil {
						return D{}
					}
					return background(gtx, colErrBg, func(gtx C) D {
						l := th.label("启动失败："+e.startErr.Error(), textSize, colErr)
						l.MaxLines = 3
						return layout.Inset{Left: 16, Right: 16, Top: 8, Bottom: 8}.Layout(gtx, l.Layout)
					})
				}),
				layout.Rigid(func(gtx C) D { return e.toolbar(gtx, changes, errs, canSave) }),
				layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }),
				layout.Rigid(func(gtx C) D {
					if e.raw || len(e.problems) == 0 {
						return D{}
					}
					gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(min(len(e.problems)*56, 168)))
					return e.issueList(gtx)
				}),
				layout.Rigid(func(gtx C) D {
					if e.toast == "" || (gtx.Now.Sub(e.toastAt) > 6*time.Second && !e.saveFailed) {
						return D{}
					}
					bg, fg := colOkBg, colOk
					if e.saveFailed {
						bg, fg = colErrBg, colErr
					}
					return background(gtx, bg, func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						txt := e.toast
						if e.saveFailed && e.saveProblem != "" {
							txt = e.saveProblem + " " + e.toast
						}
						label := th.label(txt, textSize, fg)
						label.MaxLines = 0
						return layout.Inset{Left: 16, Right: 16, Top: 7, Bottom: 7}.Layout(gtx, func(gtx C) D {
							children := []layout.FlexChild{layout.Flexed(1, label.Layout)}

							return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
						})
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					main := e.visualPane
					if e.raw {
						main = e.rawPane
					}
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, main),
						layout.Rigid(func(gtx C) D {
							if !e.showDiff {
								return D{}
							}
							return layout.Flex{}.Layout(gtx,
								layout.Rigid(func(gtx C) D { return vline(gtx, colHair) }),
								layout.Rigid(func(gtx C) D { return fixed(gtx, 340, func(gtx C) D { return e.diffPane(gtx, changes) }) }),
							)
						}),
					)
				}),
			)
		}),
	)
}

func (e *configEditor) doSave() error {
	text := e.visualYAML()
	if e.raw {
		text = e.rawEd.Text()
	}
	if e.saveFile == nil {
		return fmt.Errorf("没有可写的配置文件，请通过 -config 指定现有文件")
	}
	if err := e.saveFile(text); err != nil {
		return err
	}
	e.savedYAML = text
	e.commitBaseline()
	e.rawSynced = text
	return nil
}

func (e *configEditor) update(gtx C) {
	if e.modes[0].Clicked(gtx) && e.raw && e.rawErr == "" && !e.visualLocked {
		e.raw = false
	}
	if e.modes[1].Clicked(gtx) && !e.raw {
		e.raw = true
		e.rawSynced = e.visualYAML()
		e.rawEd.SetText(e.rawSynced)
		e.rawErr = ""
	}
	if e.discardRaw.Clicked(gtx) {
		e.rawSynced = e.visualYAML()
		e.rawEd.SetText(e.rawSynced)
		e.rawErr = ""
	}
	e.updateIssues(gtx)
	if e.diff.Clicked(gtx) {
		e.showDiff = !e.showDiff
	}
	if e.revert.Clicked(gtx) {
		if cfg, err := config.ParseDraft([]byte(e.savedYAML)); err == nil {
			e.draft = cfg
			e.rebuild()
			e.rawSynced = e.savedYAML
			e.rawEd.SetText(e.savedYAML)
			e.rawErr = ""
		}
	}

	if e.raw {
		for {
			ev, ok := e.rawEd.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				e.reparse()
			}
		}
		return
	}

	for path, b := range e.treeBtn {
		if b.Clicked(gtx) {
			e.sel = path
		}
	}
	for gi, b := range e.addDs {
		if b.Clicked(gtx) {
			gw := &e.draft.Gateways[gi]
			gw.Downstreams = append(gw.Downstreams, config.DownstreamConfig{Name: "新下游", Type: "tcp", Tcp: config.TcpConfig{Address: "192.168.1.10:502"}})
			e.rebuild()
			e.sel = fmt.Sprintf("gateways.%d.downstreams.%d", gi, len(gw.Downstreams)-1)
		}
	}
	e.updateMappings(gtx)
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if s.options != nil {
				for opt, b := range e.opts[s.path] {
					if b.Clicked(gtx) && s.get() != opt {
						s.set(opt)
						e.alignMapping(s.path, opt)
						if strings.HasSuffix(s.path, ".type") {
							e.rebuild()
						}
						e.version++
					}
				}
				continue
			}
			if ed := e.eds[s.path]; ed != nil && !s.readOnly && ed.Text() != s.get() {
				s.set(ed.Text())
				e.version++
			}
		}
	}
}

// reparse loads the raw text into the draft when it parses.
func (e *configEditor) reparse() {
	cfg, err := config.ParseDraft([]byte(e.rawEd.Text()))
	if err != nil {
		e.rawErr = err.Error()
		return
	}
	e.rawErr = ""
	e.draft = cfg
	e.rebuild()
	e.rawSynced = e.rawEd.Text()
	e.visualLocked = cfg.Version != 1
}

func (e *configEditor) toolbar(gtx C, changes []change, errs int, canSave bool) D {
	th := e.th
	return layout.Inset{Left: 16, Right: 14}.Layout(gtx, func(gtx C) D {
		children := []layout.FlexChild{
			layout.Rigid(th.bold("配置", titleSize, colInk).Layout), gap(12),
			layout.Rigid(func(gtx C) D { return th.chip(gtx, &e.modes[0], "可视化", !e.raw) }), gap(4),
			layout.Rigid(func(gtx C) D { return th.chip(gtx, &e.modes[1], "config.yaml", e.raw) }), gap(12),
			layout.Rigid(func(gtx C) D {
				switch {
				case errs > 0:
					return th.badge(gtx, fmt.Sprintf("未保存 · %d 处问题", errs), colErr, colErrBg)
				case len(changes) > 0:
					return th.badge(gtx, fmt.Sprintf("已修改 %d 处 · 未保存", len(changes)), colWarn, colWarnBg)
				}
				if e.saveFailed {
					return th.badge(gtx, "保存失败", colErr, colErrBg)
				}
				if e.savedYAML != e.runningYAML {
					return th.badge(gtx, "已保存 · 待重启", colWarn, colWarnBg)
				}
				return th.badge(gtx, "运行配置", colOk, colOkBg)
			}),
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx C) D {
				if len(changes) == 0 {
					return D{}
				}
				return layout.Inset{Right: 8}.Layout(gtx, func(gtx C) D { return th.button(gtx, &e.revert, "撤销全部", false) })
			}),
			layout.Rigid(func(gtx C) D {
				txt := "查看变更"
				if e.showDiff {
					txt = "隐藏变更"
				}
				return th.button(gtx, &e.diff, txt, false)
			}),
			gap(8),
			layout.Rigid(func(gtx C) D {
				if !canSave {
					return disabled(gtx, func(gtx C) D { return th.button(gtx, &e.save, "保存", false) })
				}
				return th.button(gtx, &e.save, "保存", false)
			}),
			gap(8),
			layout.Rigid(th.label("重启网关后生效", smallSize, colMuted).Layout),
		}
		if gtx.Constraints.Max.X < gtx.Dp(1200) {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx C) D { return row(gtx, 40, children[:7]...) }), layout.Rigid(func(gtx C) D { return row(gtx, 40, children[7:]...) }))
		}
		return row(gtx, 46, children...)
	})
}

// disabled draws w faded and ignoring input.
func disabled(gtx C, w layout.Widget) D {
	gtx = gtx.Disabled()
	d := w(gtx)
	defer clip.Rect{Max: d.Size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, alpha(colCanvas, 150))
	return d
}

// ---- visual mode ----

func (e *configEditor) visualPane(gtx C) D {
	return layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return fixed(gtx, 260, e.treePane) }),
		layout.Rigid(func(gtx C) D { return vline(gtx, colHair) }),
		layout.Flexed(1, e.formPane),
	)
}

func (e *configEditor) treePane(gtx C) D {
	th := e.th
	var items []layout.Widget
	last := ""
	for i, n := range e.nodes {
		section := map[string]string{"常规": "常规", "模拟模型": "模拟模型", "网关": "网关"}[n.kind]
		if section != "" && section != last {
			items = append(items, th.sectionTitle(section))
			last = section
		}
		items = append(items, func(gtx C) D { return e.treeItem(gtx, n) })
		if n.gw >= 0 && (i == len(e.nodes)-1 || e.nodes[i+1].gw != n.gw) {
			gi := n.gw
			items = append(items, func(gtx C) D {
				return layout.Inset{Left: 30, Top: 2, Bottom: 6}.Layout(gtx, func(gtx C) D {
					return th.button(gtx, e.addDs.get(gi), "+ 添加下游", false)
				})
			})
		}
	}
	return material.List(th.Theme, &e.tree).Layout(gtx, len(items), func(gtx C, i int) D { return items[i](gtx) })
}

func (e *configEditor) treeItem(gtx C, n *cfgNode) D {
	th := e.th
	btn := e.treeBtn.get(n.path)
	bg := colCanvas
	fg := colBody
	switch {
	case n.path == e.sel:
		bg, fg = colHover, colInk
	case btn.Hovered():
		bg = colSoft
	}
	dirty, bad := e.isNew(n), len(e.nodeProblems(n)) > 0
	for _, s := range n.specs {
		if !s.visible() {
			continue
		}
		old, ok := e.baseline[s.path]
		dirty = dirty || !ok || old != s.get()
		bad = bad || e.fieldErr(s) != ""
	}
	return layout.Inset{Left: 6, Right: 6}.Layout(gtx, func(gtx C) D {
		return btn.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return rounded(gtx, bg, 0, func(gtx C) D {
				return layout.Inset{Left: 10 + unit.Dp(n.depth)*16, Right: 8}.Layout(gtx, func(gtx C) D {
					return row(gtx, 28,
						layout.Flexed(1, func(gtx C) D {
							if n.kind == "网关" {
								return th.bold(n.title(), textSize, fg).Layout(gtx)
							}
							return th.label(n.title(), textSize, fg).Layout(gtx)
						}),
						layout.Rigid(func(gtx C) D {
							switch {
							case bad:
								return dot(gtx, colErrSolid)
							case e.isNew(n):
								return th.badge(gtx, "新增", colWarn, colWarnBg)
							case dirty:
								return dot(gtx, colWarnSolid)
							}
							return D{}
						}),
					)
				})
			})
		})
	})
}

func (e *configEditor) formPane(gtx C) D {
	th := e.th
	n := e.node(e.sel)
	var items []layout.Widget
	items = append(items, func(gtx C) D {
		return layout.Inset{Left: 24, Top: 18, Bottom: 10}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
				layout.Rigid(th.label(n.kind, smallSize, colMuted).Layout), gap(8),
				layout.Rigid(th.bold(n.title(), titleSize, colInk).Layout))
		})
	})

	for _, s := range n.specs {
		if s.visible() && !strings.Contains(s.path, ".simulation.mappings.") {
			items = append(items, func(gtx C) D { return e.fieldRow(gtx, n, s) })
		}
	}
	items = append(items, func(gtx C) D { return e.mappingGrid(gtx, n) })
	items = append(items, func(gtx C) D { return e.mappingActions(gtx, n) })
	return material.List(th.Theme, &e.form).Layout(gtx, len(items), func(gtx C, i int) D { return items[i](gtx) })
}

func (e *configEditor) fieldRow(gtx C, n *cfgNode, s *spec) D {
	th := e.th
	msg := e.fieldErr(s)
	old, had := e.baseline[s.path]
	dirty := !had || old != s.get()
	return layout.Inset{Left: 24, Right: 24, Bottom: 12}.Layout(gtx, func(gtx C) D {
		return layout.Flex{}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return fixed(gtx, 110, func(gtx C) D {
					return layout.Inset{Top: 6}.Layout(gtx, func(gtx C) D {
						return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
							layout.Rigid(th.label(s.label, textSize, colMuted).Layout),
							layout.Rigid(func(gtx C) D {
								if !dirty || e.isNew(n) {
									return D{}
								}
								return layout.Inset{Left: 4}.Layout(gtx, th.label("●", microSize, colWarn).Layout)
							}))
					})
				})
			}),
			layout.Flexed(1, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						switch {
						case s.readOnly:
							return layout.Inset{Top: 6}.Layout(gtx, th.label(s.get(), textSize, colBody).Layout)
						case s.options != nil:
							btns := e.opts[s.path]
							if btns == nil {
								btns = clicks[string]{}
								e.opts[s.path] = btns
							}
							return e.optionChips(gtx, s, btns)
						}
						ed := e.eds[s.path]
						border := colHair
						if msg != "" {
							border = colErrSolid
						} else if gtx.Focused(ed) {
							border = colAccent
						}
						gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(420))
						fieldBg := colCanvas
						if border == colErrSolid {
							fieldBg = colErrTint
						}
						return outlined(gtx, border, fieldBg, radiusSm, func(gtx C) D {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return layout.Inset{Left: 9, Right: 9, Top: 6, Bottom: 6}.Layout(gtx, func(gtx C) D {
								st := material.Editor(th.Theme, ed, "")
								st.TextSize = textSize
								st.LineHeight = uiLineHeight(textSize)
								st.LineHeightScale = 1
								return st.Layout(gtx)
							})
						})
					}),
					layout.Rigid(func(gtx C) D {
						switch {
						case msg != "":
							return layout.Inset{Top: 3}.Layout(gtx, th.label(msg, smallSize, colErr).Layout)
						case dirty && had:
							return layout.Inset{Top: 3}.Layout(gtx, th.label("原值："+orDash(old), smallSize, colWarn).Layout)
						case s.hint != "":
							label := th.label(s.hint, smallSize, colMuted)
							label.MaxLines = 0
							return layout.Inset{Top: 3}.Layout(gtx, label.Layout)
						}
						return D{}
					}),
				)
			}),
		)
	})
}

func orDash(s string) string {
	if s == "" {
		return "（空）"
	}
	return s
}

// ---- raw mode ----

func (e *configEditor) rawPane(gtx C) D {
	th := e.th
	lines := strings.Count(e.rawEd.Text(), "\n") + 1
	const lineHeight unit.Sp = 19
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return background(gtx, colSoft, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Left: 16, Right: 16, Top: 6, Bottom: 6}.Layout(gtx, th.label(
					"保存将原样写入，保留注释与格式。",
					smallSize, colMuted).Layout)
			})
		}),
		layout.Flexed(1, func(gtx C) D {
			return material.List(th.Theme, &e.rawList).Layout(gtx, 1, func(gtx C, _ int) D {
				return layout.Inset{Top: 8, Bottom: 8}.Layout(gtx, func(gtx C) D {
					return layout.Flex{}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return fixed(gtx, 52, func(gtx C) D {
								children := make([]layout.FlexChild, lines)
								for i := range children {
									children[i] = layout.Rigid(func(gtx C) D {
										// one gutter row per editor line, at the editor's line advance
										h := gtx.Sp(lineHeight)
										gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
										l := th.mono(fmt.Sprint(i+1), colMuted)
										l.Alignment = text.End
										gtx.Constraints.Min.X = gtx.Constraints.Max.X - gtx.Dp(12)
										d := l.Layout(gtx)
										d.Size.Y = h
										return d
									})
								}
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
							})
						}),
						gap(12),
						layout.Flexed(1, func(gtx C) D {
							st := material.Editor(th.Theme, &e.rawEd, "")
							st.Font = monoFont
							st.TextSize = monoSize
							st.LineHeight = lineHeight
							st.LineHeightScale = 1
							st.Color = colInk
							return st.Layout(gtx)
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }),
		layout.Rigid(e.rawProblems),
	)
}

func (e *configEditor) rawProblems(gtx C) D {
	th := e.th
	var children []layout.FlexChild
	switch {
	case e.rawErr != "":
		children = append(children,
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return th.chip(gtx, e.issueBtn.get("$parse"), "定位解析错误", false) }), gap(8),
					layout.Flexed(1, func(gtx C) D {
						_, description := e.parseDiagnostic()
						l := th.label(description, smallSize, colErr)
						l.MaxLines = 3
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D { return th.button(gtx, &e.discardRaw, "放弃文本修改", false) }))
			}),
			layout.Rigid(th.label("修正前不能保存，也不能切回可视化；表单保留最后一次能解析的内容。", smallSize, colMuted).Layout))
	case e.visualLocked:
		children = append(children, layout.Rigid(th.label("v0 配置保留原文编辑；可视化仅支持 v1，不自动升级。", smallSize, colWarn).Layout))
	case len(e.problems) > 0:
		children = append(children, layout.Rigid(th.bold(fmt.Sprintf("%d 处问题", len(e.problems)), textSize, colErr).Layout))
		for _, p := range e.problems {
			children = append(children, layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return th.chip(gtx, e.issueBtn.get(problemPath(p)), "定位 · "+e.deepest(problemPath(p)).title(), false)
					}),
					layout.Flexed(1, th.label(e.humanProblem(p), smallSize, colErr).Layout))
			}))
		}
	default:
		children = append(children, layout.Rigid(th.label("✓ 校验通过", textSize, colOk).Layout))
	}
	return layout.Inset{Left: 16, Right: 16, Top: 8, Bottom: 10}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

// ---- changes and restart ----

func (e *configEditor) diffPane(gtx C, changes []change) D {
	th := e.th
	var items []layout.Widget
	title := "待保存的变更"
	if len(changes) == 0 && e.savedYAML != e.runningYAML {
		changes = e.pendingChanges()
		title = "待应用的变更"
	}
	items = append(items, th.sectionTitle(fmt.Sprintf("%s · %d", title, len(changes))))
	if len(changes) == 0 {
		items = append(items, func(gtx C) D {
			return layout.Inset{Left: 14}.Layout(gtx, th.label("没有未保存的修改", smallSize, colMuted).Layout)
		})
	}
	for _, c := range changes {
		items = append(items, func(gtx C) D {
			title := c.label
			if c.node != nil {
				title = c.node.kind + " " + c.node.title() + " · " + c.label
				if c.node.kind == "常规" {
					title = "全局 · " + c.label
				}
			}
			return layout.Inset{Left: 14, Right: 14, Bottom: 8}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(th.label(title, smallSize, colMuted).Layout),
					layout.Rigid(func(gtx C) D {
						if c.old == "" && !c.removed {
							return D{}
						}
						return th.mono("- "+orDash(c.old), colErr).Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						if c.removed {
							return D{}
						}
						return th.mono("+ "+orDash(c.new), colOk).Layout(gtx)
					}),
				)
			})
		})
	}
	items = append(items, func(gtx C) D {
		return layout.Inset{Left: 14, Right: 14, Top: 6}.Layout(gtx, th.label("需要重启："+e.affected(changes), smallSize, colBody).Layout)
	})
	return material.List(th.Theme, &e.side).Layout(gtx, len(items), func(gtx C, i int) D { return items[i](gtx) })
}

// affected names the gateways a change set restarts; global, simulation
// and structural changes restart them all.
func (e *configEditor) affected(changes []change) string {
	seen := map[string]bool{}
	add := func(name string) { seen[name] = true }
	for _, c := range changes {
		if c.node == nil || c.node.kind == "常规" {
			return "全部网关"
		}
		if c.node.gw >= 0 {
			add(e.draft.Gateways[c.node.gw].Name)
			continue
		}
		// A shared model change affects every gateway that references that model.
		for _, g := range e.draft.Gateways {
			for _, d := range g.Downstreams {
				if d.SimulationRef == c.node.title() || (c.label == "名称" && d.SimulationRef == c.old) {
					add(g.Name)
				}
			}
		}
	}
	var names []string
	for _, g := range e.draft.Gateways {
		if seen[g.Name] {
			names = append(names, g.Name)
		}
	}
	if len(names) == 0 {
		return "无网关受影响"
	}
	return strings.Join(names, "、")
}
