package workspace

import (
	"fmt"
	"github.com/ffutop/modbus-gateway/internal/config"
	"gopkg.in/yaml.v3"
	"strings"
	"time"
)

const historyLimit = 100

type editRecord struct {
	label         string
	before, after *structureSnapshot
}

func actionLabel(action string) string {
	if s, ok := map[string]string{"add-simulation": "新增模拟模型", "new-model": "新建并绑定模型", "add-gateway": "新增网关", "add-upstream": "新增上游", "add-downstream": "新增下游", "rename": "模型改名与引用更新", "delete": "删除对象与迁移引用", "cascade": "级联删除"}[action]; ok {
		return s
	}
	return action
}
func (e *configEditor) beginEdit() {
	if e.editBase != nil {
		return
	}
	e.editBase = e.snapshot()
	e.editBase.buffers = nil
	// Raw typing is captured before reparsing, but the editor already holds the
	// new text when the ChangeEvent is delivered.
	if e.raw {
		e.editBase.text = e.rawSynced
	}
}
func (e *configEditor) recordEdit(before *structureSnapshot, label string) {
	if e.editBase != nil {
		before = e.editBase
	}
	after := e.snapshot()
	e.editBase = nil
	if before.text == after.text && len(before.buffers) == 0 && len(after.buffers) == 0 {
		return
	}
	e.history = append(e.history, editRecord{label, before, after})
	if len(e.history) > historyLimit {
		e.history = e.history[len(e.history)-historyLimit:]
	}
	e.future = nil
}
func (e *configEditor) finishEdits() error {
	if e.editBase == nil && !e.pendingRename() {
		return nil
	}
	before := e.snapshot()
	if e.editBase != nil {
		before = e.editBase
	}
	if e.raw {
		e.recordEdit(before, "编辑 YAML")
		return nil
	}
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if s.check == nil {
				continue
			}
			if !s.visible() {
				ed := e.eds[s.path]
				if ed == nil || ed.Text() == s.get() {
					continue
				}
			}
			typed := strings.HasSuffix(s.path, ".baud_rate") || strings.Contains(s.path, ".mappings.") && !strings.HasSuffix(s.path, ".table") || strings.HasSuffix(s.path, ".timeout") || strings.HasSuffix(s.path, ".rqst_pause") || strings.Contains(s.path, ".delay_rts_")
			if !typed {
				continue
			}
			v := s.get()
			if ed := e.eds[s.path]; ed != nil {
				v = ed.Text()
			}
			if msg := s.check(v); msg != "" {
				return fmt.Errorf("%s / %s：%s", n.title(), s.label, msg)
			}
		}
	}
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if s.visible() && !(n.kind == "模拟模型" && strings.HasSuffix(s.path, ".name")) {
				if ed := e.eds[s.path]; ed != nil && !s.readOnly && s.options == nil {
					s.set(ed.Text())
				}
			}
		}
	}
	root, err := parseNode(e.visualYAML())
	if err != nil {
		return err
	}
	renames := map[string]string{}
	names := map[string]bool{}
	for _, n := range e.nodes {
		if n.kind != "模拟模型" {
			continue
		}
		name := n.title()
		if ed := e.eds[n.path+".name"]; ed != nil {
			name = strings.TrimSpace(ed.Text())
		}
		if name == "" || names[name] {
			return fmt.Errorf("模型名称不能为空或重复")
		}
		names[name] = true
		renames[n.title()] = name
		putText(root, n.path+".name", name)
	}
	for gi, g := range e.draft.Gateways {
		for di, d := range g.Downstreams {
			if d.Type == "local" || d.Type == "injector" {
				if name, ok := renames[d.SimulationRef]; ok {
					putText(root, fmt.Sprintf("gateways.%d.downstreams.%d.simulation.ref", gi, di), name)
				}
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
	// Rebinding models must not introduce an injector target collision. Compare
	// with the transaction baseline so unrelated incomplete objects can remain.
	baseline, _ := config.ParseDraft([]byte(before.text))
	known := map[string]bool{}
	if baseline != nil {
		for _, p := range baseline.Problems() {
			known[p.Message] = true
		}
	}
	for _, p := range cfg.Problems() {
		if !known[p.Message] && (strings.Contains(p.Message, "overlaps another injector") || strings.Contains(p.Message, "unknown simulation ref")) {
			return fmt.Errorf("无法完成关联修改：%s", p.Message)
		}
	}
	e.editBase = nil
	e.draft, e.rawSynced = cfg, string(bytes)
	e.resetControls()
	e.rebuild()
	e.rawEd.SetText(string(bytes))
	e.recordEdit(before, "完成字段与关联编辑")
	return nil
}
func (e *configEditor) undoHistory() error {
	if e.editBase != nil {
		before := e.editBase
		e.editBase = nil
		return e.restore(before)
	}
	if len(e.history) == 0 {
		return nil
	}
	item := e.history[len(e.history)-1]
	if err := e.restore(item.before); err != nil {
		return err
	}
	e.history = e.history[:len(e.history)-1]
	e.future = append(e.future, item)
	return nil
}
func (e *configEditor) redoHistory() error {
	if e.editBase != nil {
		return fmt.Errorf("请先完成当前编辑")
	}
	if len(e.future) == 0 {
		return nil
	}
	item := e.future[len(e.future)-1]
	if err := e.restore(item.after); err != nil {
		return err
	}
	e.future = e.future[:len(e.future)-1]
	e.history = append(e.history, item)
	return nil
}

// Recovery text includes incomplete numeric/name inputs without changing the
// runnable configuration or its revision.
func (e *configEditor) recoveryContent() string {
	if e.raw {
		return e.rawEd.Text()
	}
	root, err := parseNode(e.visualYAML())
	if err != nil {
		return e.visualYAML()
	}
	for _, n := range e.nodes {
		for _, s := range n.specs {
			if !s.visible() {
				continue
			}
			if ed := e.eds[s.path]; ed != nil && ed.Text() != s.get() {
				node := yamlAt(root, strings.Split(s.path, "."))
				tag := "!!str"
				if node != nil {
					tag = node.Tag
				}
				value := scalar(ed.Text())
				value.Tag = tag
				yamlPut(root, strings.Split(s.path, "."), value)
			}
		}
	}
	text, err := yaml.Marshal(root)
	if err != nil {
		return e.visualYAML()
	}
	return string(text)
}
func (e *configEditor) autosaveRecovery(now time.Time) {
	if e.saveDraft == nil {
		return
	}
	if !e.unsaved() {
		return
	}
	text := e.recoveryCacheText
	if !e.recoveryCacheValid || e.recoveryCacheVersion != e.version || e.raw {
		text = e.recoveryContent()
		e.recoveryCacheText = text
		e.recoveryCacheVersion = e.version
		e.recoveryCacheValid = true
	}
	if text != e.lastRecovery {
		e.lastRecovery = text
		e.recoveryChanged = now
		return
	}
	if !e.recoveryChanged.IsZero() && now.Sub(e.recoveryChanged) >= 750*time.Millisecond {
		e.saveDraft(text)
		e.recoveryChanged = time.Time{}
	}
}
