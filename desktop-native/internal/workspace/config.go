// Package workspace implements the approved desktop workspace over live gateway data.
package workspace

import (
	"errors"
	"fmt"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/configfile"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"image"
	"net"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/ffutop/modbus-gateway/internal/config"
)

// spec is one editable value of the draft, addressed by a path in the same
// form as config.Problem paths ("gateways.0.downstreams.1.slave_ids").
type spec struct {
	advanced bool
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
	id    string // editor-only identity, never serialized
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
	nodesByID                            map[string]*cfgNode
	pendingDiff                          []change
	pendingDiffSaved, pendingDiffRunning string
	pendingDiffValid                     bool
	showIssues                           bool
	focusedField                         string
	changesCache                         []change
	changesCacheVersion                  int
	changesCacheSaved                    string
	changesCacheValid                    bool
	recoveryCacheVersion                 int
	recoveryCacheText                    string
	recoveryCacheValid                   bool
	recoveryBase                         string
	inputBlocked                         bool
	showPath                             bool
	inputText                            map[string]string
	semanticSaved, semanticRunning       string
	semanticValid, semanticPending       bool
	refsVersion                          int
	refsCache                            map[string][]string
	conflict                             *configfile.Conflict
	rebaseFile                           func(string) error
	conflictView                         string
	conflictEditor                       widget.Editor
	runtime                              live.Runtime
	pendingSaved, pendingRunning         string
	pendingCache                         map[string]bool
	diffRunning                          bool
	diffModes                            [2]widget.Clickable
	creation                             *configCreation
	wb                                   workbenchState
	dialogList                           widget.List
	history                              []editRecord
	future                               []editRecord
	editBase                             *structureSnapshot
	saveApply                            widget.Clickable
	applyRequested                       bool
	recoveryText                         string
	recoveryConflict                     bool
	saveDraft                            func(string)
	clearDraft                           func() error
	recoveryError                        func() error
	lastRecovery                         string
	recoveryChanged                      time.Time
	th                                   *Theme
	saveFile                             func(string) error
	openFile                             func(string) error
	saveAsFile                           func(path, text string) error
	files                                *fileDialog
	pickFile                             func(save bool, dir, name string) (string, error)
	picking                              *filePick
	toastErr                             bool
	openBtn, saveAsBtn                   widget.Clickable
	newFile                              bool
	configPath                           string
	startErr                             error
	running                              bool

	savedIDs        map[string]string
	runningIDs      map[string]string
	ids             map[string]string
	nextID          int
	baseKinds       map[string]string
	basePaths       map[string]string
	structure       clicks[string]
	deletePath      string
	replacements    map[string]string
	structureErr    string
	structureNotice string
	undoState       *structureSnapshot
	undoAfter       *structureSnapshot
	undoFields      map[string]string
	undoBtn         widget.Clickable
	renameBtn       widget.Clickable
	draft           *config.Config
	baseline        map[string]string // path → value at the last save
	baseNode        map[string]string // node path → title at the last save
	savedYAML       string
	runningYAML     string
	saveFailed      bool
	saveProblem     string
	issueBtn        clicks[string]
	mapAdd          clicks[string]
	mapRemove       clicks[string]
	visualLocked    bool
	nodes           []*cfgNode
	sel             string // selected node path
	eds             map[string]*widget.Editor
	opts            map[string]clicks[string]
	treeBtn         clicks[string]

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
	e := &configEditor{structure: clicks[string]{}, runtime: info.Runtime, rebaseFile: info.Rebase, th: th, saveFile: info.Save, openFile: info.Open, pickFile: info.PickFile, saveAsFile: info.SaveAs, newFile: info.NewFile, toast: info.Notice, configPath: info.Config.Path, startErr: info.StartErr, running: info.Running,
		inputText: map[string]string{}, eds: map[string]*widget.Editor{}, opts: map[string]clicks[string]{}, treeBtn: clicks[string]{}}
	e.tree.Axis, e.form.Axis, e.side.Axis, e.rawList.Axis = layout.Vertical, layout.Vertical, layout.Vertical, layout.Vertical
	e.issueScroll.Axis = layout.Vertical
	e.draft, _ = config.ParseDraft([]byte(info.Content))
	blank := strings.TrimSpace(info.Content) == "" && info.StartErr == nil
	if blank {
		e.draft, _ = config.ParseDraft([]byte("version: 1\n"))
	}
	if e.draft == nil {
		e.draft = &config.Config{}
	}
	e.initWorkbench(info)
	e.savedYAML, e.rawSynced = info.Content, info.Content
	e.rawEd.SetText(info.Content)
	e.rebuild()
	e.commitBaseline()
	e.sel = "group:网关"
	if info.Running {
		e.runningYAML = info.Content
		if info.RunningContent != "" {
			e.runningYAML = info.RunningContent
		}
		e.runningIDs = copyIDs(e.ids)
	}
	e.issueBtn, e.mapAdd, e.mapRemove = clicks[string]{}, clicks[string]{}, clicks[string]{}
	if info.RunningContent != "" && info.RunningContent != info.Content {
		old := e.draft
		e.draft, _ = config.ParseDraft([]byte(info.RunningContent))
		if e.draft != nil {
			e.reconcileRawIDs()
			e.runningIDs = copyIDs(e.ids)
		}
		e.draft = old
		e.ids = copyIDs(e.savedIDs)
		e.rebuild()
	}
	e.checked = -1
	e.raw = info.Config.Version != 1 || info.StartErr != nil
	e.visualLocked = info.Config.Version != 1
	if blank {
		e.raw, e.visualLocked = false, false
	} else if _, err := config.ParseDraft([]byte(info.Content)); err != nil {
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
	p(0, "pprof:")
	p(1, "enabled: %t", c.Pprof.Enabled)
	p(1, "address: %s", q(c.Pprof.Address))
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
		writeSerial(p, indent+1, serial)
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
	if e.inputText == nil {
		e.inputText = map[string]string{}
	}
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
	uiOn.advanced, uiListen.advanced = true, true
	uiOn.label = "管理 API（CLI）"
	uiOn.hint = "仅用于直接 CLI 运行；桌面管理连接由 sidecar 覆盖"
	uiListen.hint = "桌面版由子进程管理 API 监听地址；此项用于命令行运行"
	g.specs = []*spec{level, file, uiOn, uiListen}
	g.specs = append(g.specs, globalAdvanced(c)...)
	nodes = append(nodes, g)

	for i := range c.Simulations {
		s := &c.Simulations[i]
		base := fmt.Sprintf("simulations.%d", i)
		n := &cfgNode{path: base, kind: "模拟模型", title: func() string { return s.Name }, gw: -1}
		name := strSpec(base+".name", "名称", &s.Name)
		name.check = checkName
		name.hint = "完成编辑时同步更新所有引用"
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
		gn := &cfgNode{path: base, kind: "网关", title: func() string {
			if gw.Name == "" {
				return fmt.Sprintf("网关 %d", i+1)
			}
			return gw.Name
		}, gw: i}
		name := strSpec(base+".name", "名称", &gw.Name)
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
			baud := baudSpec(ub+".serial.baud_rate", &u.Serial.BaudRate)
			baud.when = ud.when
			un.specs = []*spec{ut, ua, ud, baud}
			un.specs = append(un.specs, serialSpecs(ub, &u.Serial, ud.when)...)
			nodes = append(nodes, un)
		}
		for j := range gw.Downstreams {
			d := &gw.Downstreams[j]
			db := fmt.Sprintf("%s.downstreams.%d", base, j)
			dn := &cfgNode{path: db, kind: "下游", depth: 1, title: func() string { return d.DisplayName(j) }, gw: i}
			name := strSpec(db+".name", "名称", &d.Name)
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
			baud := baudSpec(db+".serial.baud_rate", &d.Serial.BaudRate)
			baud.when = serial
			ref := strSpec(db+".simulation.ref", "模拟模型", &d.SimulationRef)
			ref.options, ref.when = simNames, model
			ref.hint = "多个下游可以引用同一个模型，共享数据"
			dn.specs = []*spec{name, dt, ids, da, dd, baud, ref}
			dn.specs = append(dn.specs, serialSpecs(db, &d.Serial, serial)...)
			if d.Type == "injector" {
				for mi := range d.Mappings {
					m := &d.Mappings[mi]
					mb := fmt.Sprintf("%s.simulation.mappings.%d", db, mi)
					st := strSpec(mb+".source.table", fmt.Sprintf("映射 %d · 源表", mi+1), &m.Source.Table)
					st.options = []string{"coils", "holding_registers"}
					tt := strSpec(mb+".target.table", "目标表", &m.Target.Table)
					tt.options = []string{"input_registers"}
					if m.Source.Table == "coils" {
						tt.options = []string{"discrete_inputs"}
					}
					dn.specs = append(dn.specs, st, uintSpec(mb+".source.start_address", "源地址", &m.Source.StartAddress), uintSpec(mb+".source.count", "数量", &m.Source.Count), tt, uintSpec(mb+".target.start_address", "目标地址", &m.Target.StartAddress))
				}
			}
			nodes = append(nodes, dn)
		}
	}
	if e.ids == nil {
		e.ids = map[string]string{}
	}
	for _, n := range nodes {
		if e.ids[n.path] == "" {
			e.nextID++
			e.ids[n.path] = fmt.Sprintf("entity-%d", e.nextID)
		}
		n.id = e.ids[n.path]
	}
	e.nodes = nodes
	e.nodesByID = map[string]*cfgNode{}
	for _, n := range nodes {
		e.nodesByID[n.id] = n
	}
	for _, n := range nodes {
		for _, s := range n.specs {
			if s.options != nil || s.readOnly {
				continue
			}
			ed := e.eds[s.path]
			wasNil := ed == nil
			pendingName := ed != nil && n.kind == "模拟模型" && strings.HasSuffix(s.path, ".name") && ed.Text() != s.get()
			if ed == nil {
				ed = &widget.Editor{SingleLine: true}
				e.eds[s.path] = ed
			}
			if ed.Text() != s.get() && !pendingName && (e.editBase == nil || wasNil) {
				ed.SetText(s.get())
			}
		}
	}
	if e.node(e.sel) == nil && !strings.HasPrefix(e.sel, "group:") {
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
	e.baseKinds, e.basePaths = map[string]string{}, map[string]string{}
	for _, n := range e.nodes {
		e.baseNode[n.id], e.baseKinds[n.id], e.basePaths[n.id] = n.title(), n.kind, n.path
		for _, s := range n.specs {
			if s.visible() {
				e.baseline[e.fieldKey(n, s)] = s.get()
			}
		}
	}
	e.undoState = nil
	e.savedIDs = copyIDs(e.ids)
}
func (e *configEditor) fieldKey(n *cfgNode, s *spec) string {
	return n.id + "." + strings.TrimPrefix(s.path, n.path+".")
}
func (e *configEditor) changes() []change {
	var out []change
	seen := map[string]bool{}
	fields := map[string]bool{}
	for _, n := range e.nodes {
		seen[n.id] = true
		if _, exists := e.baseNode[n.id]; !exists {
			out = append(out, change{node: n, label: "新增" + n.kind, new: n.title()})
			continue
		}
		for _, s := range n.specs {
			if !s.visible() {
				continue
			}
			fields[e.fieldKey(n, s)] = true
			old, ok := e.baseline[e.fieldKey(n, s)]
			if !ok || old != s.get() {
				out = append(out, change{node: n, label: s.label, old: old, new: s.get()})
			}
		}
	}
	byID := map[string]*cfgNode{}
	for _, n := range e.nodes {
		byID[n.id] = n
	}
	removedMappings := map[string]bool{}
	for field := range e.baseline {
		if fields[field] {
			continue
		}
		id, _, ok := strings.Cut(field, ".simulation.mappings.")
		if !ok || removedMappings[id] {
			continue
		}
		if n := byID[id]; n != nil {
			if d := e.downstream(n.path); d != nil && d.Type == "injector" {
				out = append(out, change{node: n, label: "移除映射", old: "原映射集合", new: fmt.Sprintf("剩余 %d 条映射", len(d.Mappings))})
				removedMappings[id] = true
			}
		}
	}
	for id, title := range e.baseNode {
		if !seen[id] {
			// A gateway removal already includes its children.
			parent := strings.Split(e.basePaths[id], ".")
			if len(parent) > 2 {
				hidden := false
				for pid, path := range e.basePaths {
					if path == strings.Join(parent[:2], ".") && !seen[pid] {
						hidden = true
					}
				}
				if hidden {
					continue
				}
			}
			out = append(out, change{label: "删除" + e.baseKinds[id], old: title, removed: true})
		}
	}
	return out
}

func (e *configEditor) isNew(n *cfgNode) bool {
	_, ok := e.baseNode[n.id]
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
		ed := e.eds[s.path]
		if ed == nil || ed.Text() == s.get() {
			return ""
		}
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
	fields := map[string]bool{}
	for _, p := range e.problems {
		fields[problemPath(p)] = true
	}
	if !e.raw {
		for _, node := range e.nodes {
			for _, s := range node.specs {
				if s.check != nil && e.fieldErr(s) != "" {
					fields[s.path] = true
				}
			}
		}
	}
	return len(fields)
}

// ---- frame ----

// unsaved reports edits not yet written to the file.
func (e *configEditor) unsaved() bool {
	return e.editBase != nil || len(e.draftChanges()) > 0 || e.pendingRename() || e.raw && e.rawEd.Text() != e.savedYAML
}

func (e *configEditor) Layout(gtx C) D {
	th := e.th
	if !e.inputBlocked {
		e.update(gtx)
		e.updateWorkbench(gtx)
	}
	e.autosaveRecovery(gtx.Now)
	if e.checked != e.version {
		e.problems, e.checked = e.validationProblems(), e.version
	}
	changes := append([]change(nil), e.draftChanges()...)
	if e.pendingRename() {
		for _, n := range e.nodes {
			if n.kind == "模拟模型" {
				if ed := e.eds[n.path+".name"]; ed != nil && ed.Text() != n.title() {
					changes = append(changes, change{node: n, label: "待应用改名", old: n.title(), new: ed.Text()})
				}
			}
		}
	}
	if e.raw && e.rawEd.Text() != e.savedYAML && len(changes) == 0 {
		changes = append(changes, change{label: "文件文本", old: e.savedYAML, new: e.rawEd.Text()})
	}
	errs := e.errorCount()
	canSave := len(changes) > 0 && errs == 0
	for i := range e.diffModes {
		if e.diffModes[i].Clicked(gtx) {
			e.diffRunning = i == 1
		}
	}
	apply := e.saveApply.Clicked(gtx) && !e.inputBlocked
	if apply && !e.unsaved() && (e.needsApply() || !e.isRunning()) && errs == 0 {
		e.applyRequested = true
		gtx.Execute(op.InvalidateCmd{})
	}
	if (e.save.Clicked(gtx) || apply) && !e.inputBlocked && canSave && !e.isStarting() {
		e.toastErr = false
		if err := e.doSave(); err != nil {
			e.saveFailed, e.saveProblem = true, err.Error()
			e.toast, e.toastAt = "保存失败，草稿和运行配置保留。", gtx.Now
		} else {
			e.saveFailed, e.saveProblem = false, ""
			e.toast, e.toastAt = "已保存。重启网关后使用新配置。", gtx.Now
			e.applyRequested = apply && (e.needsApply() || !e.isRunning())
			if apply {
				gtx.Execute(op.InvalidateCmd{})
			}
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

				layout.Rigid(e.workbenchBar),
				layout.Rigid(e.recoveryBanner),
				layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }),
				layout.Rigid(func(gtx C) D {
					if e.raw || len(e.problems) == 0 {
						return D{}
					}
					return e.issueSummary(gtx)
				}),
				layout.Rigid(func(gtx C) D {
					if e.toast != "" && e.toastAt.IsZero() {
						e.toastAt = gtx.Now
					}
					if e.toast == "" || (gtx.Now.Sub(e.toastAt) > 6*time.Second && !e.saveFailed) {
						return D{}
					}
					bg, fg := colOkBg, colOk
					if e.saveFailed || e.toastErr {
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
		layout.Stacked(e.configPopupDismiss),
		layout.Stacked(e.configFilterPopup),
	)
}

// draftText finishes pending edits and returns the draft as a valid file.
func (e *configEditor) draftText() (string, error) {
	if err := e.finishEdits(); err != nil {
		return "", err
	}
	text := e.visualYAML()
	if e.raw {
		text = e.rawEd.Text()
	}
	cfg, err := config.ParseDraft([]byte(text))
	if err != nil {
		return "", err
	}
	if ps := cfg.Problems(); len(ps) > 0 {
		return "", fmt.Errorf("%s", ps[0].Message)
	}
	return text, nil
}

func (e *configEditor) doSave() error {
	text, err := e.draftText()
	if err != nil {
		return err
	}
	if e.saveFile == nil {
		return fmt.Errorf("没有可写的配置文件，请通过 -config 指定现有文件")
	}
	if err := e.saveFile(text); err != nil {
		if errors.As(err, &e.conflict) {
			e.conflictView = "草稿"
			e.conflictEditor.SetText(text)
		}
		return err
	}
	e.savedYAML = text
	e.newFile = false
	e.commitBaseline()
	e.rawSynced = text
	if e.clearDraft != nil {
		if err := e.clearDraft(); err != nil {
			e.structureErr = "配置已保存，但草稿清理失败：" + err.Error()
		}
	}
	e.lastRecovery = text
	return nil
}

func (e *configEditor) update(gtx C) {
	if e.modes[0].Clicked(gtx) && e.raw && e.rawErr == "" && !e.visualLocked {
		e.raw = false
		e.version++
	}
	if e.modes[1].Clicked(gtx) && !e.raw {
		if e.pendingRename() {
			e.structureErr = "请先完成模型改名或撤销输入，再切换 YAML"
		} else if err := e.finishEdits(); err != nil {
			e.structureErr = err.Error()
		} else {
			e.raw = true
			e.version++
			e.rawSynced = e.visualYAML()
			if !e.unsaved() {
				e.rawSynced = e.savedYAML
			}
			e.rawEd.SetText(e.rawSynced)
			e.rawErr = ""
		}
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
	if e.openBtn.Clicked(gtx) && e.openFile != nil {
		e.chooseFile(gtx, false)
	}
	if e.saveAsBtn.Clicked(gtx) && e.saveAsFile != nil && e.errorCount() == 0 {
		e.chooseFile(gtx, true)
	}
	if e.revert.Clicked(gtx) {
		if cfg, err := config.ParseDraft([]byte(e.savedYAML)); err == nil {
			e.editBase = nil
			e.history = nil
			e.future = nil
			e.draft = cfg
			selectedID := e.ids[e.sel]
			e.sel = "global"
			for path, id := range e.savedIDs {
				if id == selectedID {
					e.sel = path
					break
				}
			}
			e.ids = copyIDs(e.savedIDs)
			e.resetControls()
			e.undoState = nil
			e.rebuild()
			e.rawSynced = e.savedYAML
			e.rawEd.SetText(e.savedYAML)
			e.rawErr = ""
			if e.clearDraft != nil {
				_ = e.clearDraft()
			}
		}
	}

	if e.raw {
		for {
			ev, ok := e.rawEd.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok && e.rawEd.Text() != e.rawSynced {
				e.beginEdit()
				e.reparse()
			}
		}
		return
	}

	for path, b := range e.treeBtn {
		if b.Clicked(gtx) {
			e.navigate(path)
		}
	}
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if s.options != nil {
				for opt, b := range e.opts[s.path] {
					if b.Clicked(gtx) {
						e.wb.optionPath = ""
						if s.get() == opt {
							continue
						}
						if strings.HasSuffix(s.path, ".type") {
							path := s.path
							// Flush the active protocol's editors before hiding them. The next
							// transaction records the protocol switch and preserves inactive YAML.
							e.beginEdit()
							if err := e.finishEdits(); err != nil {
								e.structureErr = err.Error()
								return
							}
							for _, node := range e.nodes {
								for _, field := range node.specs {
									if field.path == path {
										s = field
									}
								}
							}
							e.beginEdit()
							s.set(opt)
							e.transportDefaults(path, opt)
							e.rebuild()
							return
						}
						e.beginEdit()
						s.set(opt)
						e.transportDefaults(s.path, opt)
						e.alignMapping(s.path, opt)
						if strings.HasSuffix(s.path, ".source.table") {
							e.rebuild()
							return
						}
						if strings.HasSuffix(s.path, ".type") {
							e.rebuild()
						}
						e.version++
					}
				}
				continue
			}
			if n.kind == "模拟模型" && strings.HasSuffix(s.path, ".name") {
				if ed := e.eds[s.path]; ed != nil && ed.Text() != s.get() && e.inputText[s.path] != ed.Text() {
					e.beginEdit()
					e.inputText[s.path] = ed.Text()
					e.version++
				}
				continue
			}
			if ed := e.eds[s.path]; ed != nil && !s.readOnly && ed.Text() != s.get() && e.inputText[s.path] != ed.Text() {
				e.beginEdit()
				s.set(ed.Text())
				e.inputText[s.path] = ed.Text()
				e.version++
			}
		}
	}
	if e.updateStructure(gtx) {
		return
	}
	e.updateMappings(gtx)
	e.finishUnfocusedField(gtx)
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
	if saved, err := config.ParseDraft([]byte(e.savedYAML)); err == nil && reflect.DeepEqual(saved, cfg) {
		e.ids = copyIDs(e.savedIDs)
	} else {
		e.reconcileRawIDs()
	}
	e.resetControls()
	e.undoState = nil
	e.rebuild()
	e.rawSynced = e.rawEd.Text()
	e.visualLocked = cfg.Version != 1
}

func (e *configEditor) toolbar(gtx C, changes []change, errs int, canSave bool) D {
	th := e.th
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 20, Right: 20, Top: 10, Bottom: 10}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										gtx.Constraints.Max.X = max(gtx.Dp(80), gtx.Constraints.Max.X-gtx.Dp(210))
										label := th.bold(filepath.Base(e.configPath), textSize, colInk)
										label.MaxLines = 1
										return label.Layout(gtx)
									}), gap(8),
									layout.Rigid(func(gtx C) D { return e.configBadge(gtx, fmt.Sprintf("v%d", e.draft.Version), colBody, colCard) }), gap(8),
									layout.Rigid(func(gtx C) D {
										if !e.newFile {
											return D{}
										}
										return layout.Inset{Right: 8}.Layout(gtx, func(gtx C) D { return e.configBadge(gtx, "新文件 · 保存时创建", colWarn, colWarnBg) })
									}),
									layout.Rigid(func(gtx C) D { return e.configToolbarState(gtx, changes, errs) }))
							}), vgap(4), layout.Rigid(func(gtx C) D { l := th.label(e.configPath, smallSize, colMuted); l.MaxLines = 1; return l.Layout(gtx) }))
					}), gap(16),
					layout.Rigid(func(gtx C) D { return e.configButton(gtx, &e.openBtn, "打开…", btnDefault) }), gap(8),
					layout.Rigid(func(gtx C) D {
						draw := func(gtx C) D { return e.configButton(gtx, &e.saveAsBtn, "另存为…", btnDefault) }
						if errs > 0 || e.saveAsFile == nil {
							return disabled(gtx, draw)
						}
						return draw(gtx)
					}), gap(16),
					layout.Rigid(func(gtx C) D {
						labels := [2]string{"可视化", "YAML"}
						return layout.Flex{}.Layout(gtx, layout.Rigid(func(gtx C) D { return e.configSegment(gtx, &e.modes[0], labels[0], !e.raw, true, false) }), layout.Rigid(func(gtx C) D { return e.configSegment(gtx, &e.modes[1], labels[1], e.raw, false, true) }))
					}), gap(8),
					layout.Rigid(func(gtx C) D { return e.configButton(gtx, &e.diff, "变更", btnDefault) }), gap(8),
					layout.Rigid(func(gtx C) D {
						if !canSave {
							return D{}
						}
						return layout.Inset{Right: 8}.Layout(gtx, func(gtx C) D { return e.configButton(gtx, &e.save, "仅保存", btnDefault) })
					}),
					layout.Rigid(func(gtx C) D {
						label := "保存并应用"
						if !e.unsaved() {
							label = "应用配置"
						}
						if !e.isRunning() {
							label = "启动网关"
						}
						if e.newFile {
							label = "保存并启动"
						}
						if e.isStarting() {
							label = "正在应用…"
						}
						if !canSave && !e.needsApply() && e.isRunning() {
							label = "配置已同步"
						}
						kind := btnDefault
						if canSave || e.needsApply() || !e.isRunning() {
							kind = btnPrimary
						}
						draw := func(gtx C) D { return e.configButton(gtx, &e.saveApply, label, kind) }
						if e.isStarting() || errs > 0 || (!canSave && !e.needsApply() && e.isRunning()) || (e.newFile && !canSave) {
							return disabled(gtx, draw)
						}
						return draw(gtx)
					}))
			})
		}), layout.Rigid(func(gtx C) D { return hline(gtx, colHair) }))
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
		layout.Rigid(func(gtx C) D { return fixed(gtx, 231, e.workbenchTree) }),
		layout.Rigid(func(gtx C) D { return vline(gtx, colHair) }),
		layout.Flexed(1, e.workbenchPane),
	)
}

func (e *configEditor) treeItem(gtx C, n *cfgNode) D {
	btn := e.treeBtn.get(n.path)
	bg, fg := colSoft, colBody
	if n.path == e.sel {
		bg, fg = colSelected, colOnSelected
	} else if btn.Hovered() {
		bg = colSoft
	}
	depth := n.depth
	if n.kind == "模拟模型" {
		depth = 1
	}
	if n.kind == "上游" || n.kind == "下游" {
		depth = 1
	}
	if n.kind == "网关" {
		depth = 0
	}
	return layout.Inset{Left: 6, Right: 6}.Layout(gtx, func(gtx C) D {

		return rounded(gtx, bg, radiusSm, func(gtx C) D {
			if depth > 0 {
				paint.FillShape(gtx.Ops, colHair, clip.Rect{Min: image.Pt(gtx.Dp(16), 0), Max: image.Pt(gtx.Dp(16)+1, gtx.Dp(34))}.Op())
			}
			return layout.Inset{Left: unit.Dp(8 + depth*20), Right: 8}.Layout(gtx, func(gtx C) D {
				return row(gtx, 34,
					layout.Rigid(func(gtx C) D {
						if n.kind == "网关" {
							return e.treeDisclosure(gtx, "collapse-id|"+n.id, !e.wb.collapsed[n.id])
						}
						return fixed(gtx, 12, func(gtx C) D { return D{} })
					}), gap(6),
					layout.Flexed(1, func(gtx C) D {
						return btn.Layout(gtx, func(gtx C) D {
							return row(gtx, 34, layout.Rigid(func(gtx C) D { return configRoleIcon(gtx, n.kind, fg) }), gap(6),
								layout.Flexed(1, func(gtx C) D {
									title := configNodeTitle(n)
									if n.kind == "常规" {
										title = "全局运行设置"
									}
									l := e.th.label(title, textSize, fg)
									l.MaxLines = 1
									return l.Layout(gtx)
								}),
								layout.Rigid(func(gtx C) D {
									if len(e.nodeProblems(n)) > 0 {
										return e.th.label("问题", smallSize, colErr).Layout(gtx)
									}
									if e.dirty(n) {
										return e.th.label("修改", smallSize, colWarn).Layout(gtx)
									}
									if e.pendingIDs()[n.id] {
										return e.th.label("待用", smallSize, colAccent).Layout(gtx)
									}
									return D{}
								}))
						})
					}))
			})
		})
	})
}

func (e *configEditor) formPane(gtx C) D {
	n := e.node(e.sel)
	if n == nil {
		return D{}
	}
	items := []layout.Widget{func(gtx C) D { return e.configDetailHeader(gtx, n) }}
	items = append(items, func(gtx C) D {
		nodes := []*cfgNode{}
		for _, child := range e.nodes {
			if n.kind == "网关" && child.gw == n.gw && child.depth > 0 {
				nodes = append(nodes, child)
			}
			if n.kind == "模拟模型" && child.kind == "下游" {
				if d := e.downstream(child.path); d != nil && d.SimulationRef == n.title() {
					nodes = append(nodes, child)
				}
			}
		}
		return layout.Inset{Left: 24, Right: 24}.Layout(gtx, func(gtx C) D { return e.configSelectionBar(gtx, nodes) })
	})
	specs := []*spec{}
	for _, s := range n.specs {
		if s.visible() && (!s.advanced || e.wb.advanced[n.id]) && !strings.Contains(s.path, ".simulation.mappings.") {
			specs = append(specs, s)
		}
	}
	items = append(items, func(gtx C) D { return e.configFieldGrid(gtx, n, specs) })
	items = append(items, func(gtx C) D { return e.objectExplanation(gtx, n) })
	if n.kind == "网关" || n.kind == "模拟模型" {
		items = append(items, func(gtx C) D { return e.configRelations(gtx, n) })
	} else {
		items = append(items, func(gtx C) D { return e.relationsPane(gtx, n) })
	}
	items = append(items, func(gtx C) D { return e.mappingGrid(gtx, n) }, func(gtx C) D { return D{} })
	if e.deletePath != "" {
		items = append(items, func(gtx C) D { return e.structureActions(gtx, n) })
	}
	return e.configList(gtx, &e.form, len(items), func(gtx C, i int) D { return items[i](gtx) })
}
func (e *configEditor) fieldRow(gtx C, n *cfgNode, s *spec) D {
	return layout.Inset{Bottom: 14}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				label := s.label
				if old, ok := e.baseline[e.fieldKey(n, s)]; ok && old != s.get() {
					label += " · 已修改"
				}
				return e.th.label(label, smallSize, colBody).Layout(gtx)
			}), vgap(4),
			layout.Rigid(func(gtx C) D { return e.configFieldControl(gtx, s) }), layout.Rigid(func(gtx C) D {
				if !strings.HasSuffix(s.path, ".simulation.ref") {
					return D{}
				}
				return layout.Inset{Top: 4}.Layout(gtx, func(gtx C) D {
					return e.configButton(gtx, e.structure.get("new-model|"+n.path), "新建并引用模型…", btnLink)
				})
			}),
			layout.Rigid(func(gtx C) D {
				msg, ink := e.fieldErr(s), colErr
				if msg == "" {
					msg, ink = s.hint, colMuted
				}
				if msg == "" {
					return D{}
				}
				return layout.Inset{Top: 4}.Layout(gtx, func(gtx C) D { l := e.th.label(msg, smallSize, ink); l.MaxLines = 0; return l.Layout(gtx) })
			}))
	})
}
func (e *configEditor) configFieldControl(gtx C, s *spec) D {
	if strings.HasSuffix(s.path, ".simulation.ref") {
		return e.modelSelector(gtx, s)
	}
	if s.readOnly {
		return e.th.label(s.get(), textSize, colBody).Layout(gtx)
	}
	if s.options != nil {
		return e.configChoice(gtx, s)
	}
	ed := e.eds[s.path]
	border := colControl
	if e.fieldErr(s) != "" {
		border = colErrSolid
	} else if gtx.Focused(ed) {
		border = colAccent
	}
	return outlined(gtx, border, colCanvas, radiusSm, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Left: 9, Right: 9, Top: 7, Bottom: 7}.Layout(gtx, func(gtx C) D {
			st := material.Editor(e.th.Theme, ed, "")
			st.TextSize = textSize
			st.LineHeight = uiLineHeight(textSize)
			st.LineHeightScale = 1
			return st.Layout(gtx)
		})
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
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 16, Top: 6, Bottom: 6}.Layout(gtx, e.th.label("YAML 配置", titleSize, colInk).Layout)
		}),
		layout.Flexed(1, func(gtx C) D {
			return layout.UniformInset(16).Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min = gtx.Constraints.Max
				st := material.Editor(e.th.Theme, &e.rawEd, "")
				st.Font, st.TextSize, st.LineHeight, st.LineHeightScale, st.Color = monoFont, monoSize, 19, 1, colInk
				return st.Layout(gtx)
			})
		}), layout.Rigid(e.rawProblems))
}

func (e *configEditor) rawProblems(gtx C) D {
	th := e.th
	var children []layout.FlexChild
	switch {
	case e.rawErr != "":
		children = append(children,
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return e.configButton(gtx, e.issueBtn.get("$parse"), "定位解析错误", btnLink) }), gap(8),
					layout.Flexed(1, func(gtx C) D {
						_, description := e.parseDiagnostic()
						l := th.label(description, smallSize, colErr)
						l.MaxLines = 3
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D { return e.configButton(gtx, &e.discardRaw, "放弃文本修改", btnDefault) }))
			}),
			layout.Rigid(th.label("修正前不能保存，也不能切回可视化；表单保留最后一次能解析的内容。", smallSize, colMuted).Layout))
	case e.visualLocked:
		children = append(children, layout.Rigid(th.label("v0 配置保留原文编辑；可视化仅支持 v1，不自动升级。", smallSize, colWarn).Layout))
	case len(e.problems) > 0:
		children = append(children, layout.Rigid(e.issueSummary))
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
	items = append(items, func(gtx C) D {
		return layout.Flex{}.Layout(gtx, th.segmented(2, func(i int) (*widget.Clickable, string, bool) {
			return &e.diffModes[i], []string{"未保存", "待应用"}[i], e.diffRunning == (i == 1)
		})...)
	})
	if e.diffRunning {
		changes = e.pendingChanges()
		title = "待应用的变更"
		if !e.needsApply() {
			items = append(items, th.label("仅文本变化或语义一致，无需运行变更。", smallSize, colMuted).Layout)
		}
	}
	items = append(items, th.sectionTitle(fmt.Sprintf("%s · %d", title, len(changes))))
	if len(changes) == 0 {
		items = append(items, func(gtx C) D {
			return layout.Inset{Left: 14}.Layout(gtx, th.label("此比较没有变更", smallSize, colMuted).Layout)
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
					layout.Rigid(func(gtx C) D {
						if c.node != nil {
							return e.wbButton(gtx, "open|"+c.node.path, title, btnLink)
						}
						return th.label(title, smallSize, colMuted).Layout(gtx)
					}),
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
		return layout.Inset{Left: 14, Right: 14, Top: 6}.Layout(gtx, th.label("变更涉及："+e.affected(changes), smallSize, colBody).Layout)
	})
	return material.List(th.Theme, &e.side).Layout(gtx, len(items), func(gtx C, i int) D { return items[i](gtx) })
}

// affected names the gateways a change set restarts; global, simulation
// and structural changes restart them all.
func (e *configEditor) affected(changes []change) string {
	cfg := e.draft
	if e.diffRunning {
		if saved, err := config.ParseDraft([]byte(e.savedYAML)); err == nil {
			cfg = saved
		}
	}
	seen := map[string]bool{}
	add := func(name string) { seen[name] = true }
	for _, c := range changes {
		if c.node == nil || c.node.kind == "常规" {
			return "全部网关"
		}
		if c.node.gw >= 0 {
			add(cfg.Gateways[c.node.gw].Name)
			continue
		}
		// A shared model change affects every gateway that references that model.
		for _, g := range cfg.Gateways {
			for _, d := range g.Downstreams {
				if d.SimulationRef == c.node.title() || (c.label == "名称" && d.SimulationRef == c.old) {
					add(g.Name)
				}
			}
		}
	}
	var names []string
	for _, g := range cfg.Gateways {
		if seen[g.Name] {
			names = append(names, g.Name)
		}
	}
	if len(names) == 0 {
		return "配置无网关引用；应用配置仍将重启全部链路"
	}
	return strings.Join(names, "、") + "（应用配置将重启全部链路）"
}

func (e *configEditor) isStarting() bool {
	return e.runtime != nil && e.runtime.State().Phase == live.Starting
}
func (e *configEditor) isRunning() bool {
	if e.runtime != nil {
		return e.runtime.State().Phase == live.Running
	}
	return e.running
}
func (e *configEditor) needsApply() bool {
	if e.semanticValid && e.semanticSaved == e.savedYAML && e.semanticRunning == e.runningYAML {
		return e.semanticPending
	}
	e.semanticSaved, e.semanticRunning, e.semanticValid = e.savedYAML, e.runningYAML, true
	a, err := config.ParseDraft([]byte(e.savedYAML))
	if err != nil {
		e.semanticPending = false
		return false
	}
	b, err := config.ParseDraft([]byte(e.runningYAML))
	e.semanticPending = err != nil || !reflect.DeepEqual(a, b)
	return e.semanticPending
}

// Core validity remains authoritative for YAML. New visual objects additionally
// need usable links; existing CLI-supported empty gateways remain editable.
func (e *configEditor) validationProblems() []config.Problem {
	if e.raw {
		return e.draft.Problems()
	}
	ps := editorProblems(e.draft)
	out := ps[:0]
	for _, p := range ps {
		if strings.Contains(p.Message, "至少需要") {
			n := e.deepest(problemPath(p))
			if n != nil && !e.isNew(n) {
				continue
			}
		}
		out = append(out, p)
	}
	paths := map[string]bool{}
	for _, p := range out {
		paths[problemPath(p)] = true
	}
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if s.check == nil || paths[s.path] {
				continue
			}
			ed := e.eds[s.path]
			if !s.visible() && (ed == nil || ed.Text() == s.get()) {
				continue
			}
			value := s.get()
			if ed != nil {
				value = ed.Text()
			}
			if msg := s.check(value); msg != "" {
				parts := strings.Split(s.path, ".")
				path := make([]any, len(parts))
				for i, p := range parts {
					if index, err := strconv.Atoi(p); err == nil {
						path[i] = index
					} else {
						path[i] = p
					}
				}
				out = append(out, config.Problem{Path: path, Message: msg})
				paths[s.path] = true
			}
		}
	}
	return out
}

func (e *configEditor) draftChanges() []change {
	if !e.changesCacheValid || e.changesCacheVersion != e.version || e.changesCacheSaved != e.savedYAML {
		e.changesCache = e.changes()
		e.changesCacheVersion = e.version
		e.changesCacheSaved = e.savedYAML
		e.changesCacheValid = true
	}
	return e.changesCache
}

func (e *configEditor) finishUnfocusedField(gtx C) {
	previous := e.focusedField
	e.focusedField = ""
	for path, ed := range e.eds {
		if gtx.Focused(ed) {
			e.focusedField = path
			break
		}
	}
	if previous == "" || previous == e.focusedField || e.editBase == nil || e.pendingRename() || strings.Contains(previous, ".simulation.") {
		return
	}
	if err := e.finishEdits(); err != nil {
		e.structureErr = err.Error()
	}
}
