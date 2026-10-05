package workspace

import (
	"fmt"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"strings"
)

type configCreation struct {
	focused                            bool
	action, path, persistence, problem string
	name, location                     widget.Editor
	clicks                             clicks[string]
}

func (e *configEditor) openCreation(action, path string) {
	c := &configCreation{action: action, path: path, persistence: "memory", clicks: clicks[string]{}}
	c.name.SingleLine, c.location.SingleLine = true, true
	e.creation = c
}

func (e *configEditor) confirmCreation() error {
	c := e.creation
	if c == nil {
		return nil
	}
	name := strings.TrimSpace(c.name.Text())
	if name == "" {
		return fmt.Errorf("请输入名称")
	}
	for _, n := range e.nodes {
		if ((c.action == "add-gateway" && n.kind == "网关") || (c.action != "add-gateway" && n.kind == "模拟模型")) && n.title() == name {
			return fmt.Errorf("名称已存在")
		}
	}
	if c.action != "add-gateway" && c.persistence != "memory" && strings.TrimSpace(c.location.Text()) == "" {
		return fmt.Errorf("请输入持久化路径或连接串")
	}
	if c.action == "new-model" {
		d := e.downstream(c.path)
		if d == nil || (d.Type != "local" && d.Type != "injector") {
			return fmt.Errorf("请选择 local 或 injector 下游")
		}
	}
	if err := e.mutateStructure(c.action, c.path, map[string]string{"name": name, "persistence": c.persistence, "path": c.location.Text()}); err != nil {
		return err
	}
	e.creation = nil
	e.wb.picker = ""
	return nil
}

func (e *configEditor) updateCreation(gtx C) {
	c := e.creation
	if c == nil {
		return
	}
	for _, ed := range []*widget.Editor{&c.name, &c.location} {
		for {
			if _, ok := ed.Update(gtx); !ok {
				break
			}
		}
	}
	for action, b := range c.clicks {
		if !b.Clicked(gtx) {
			continue
		}
		switch action {
		case "cancel":
			e.creation = nil
			gtx.Execute(key.FocusCmd{Tag: e.structure.get(c.action + "|" + c.path)})
		case "confirm":
			if err := e.confirmCreation(); err != nil {
				c.problem = err.Error()
			}
		default:
			c.persistence = action
			c.problem = ""
		}
	}
}

func (e *configEditor) creationPane(gtx C) D {
	c := e.creation
	if !c.focused {
		gtx.Execute(key.FocusCmd{Tag: &c.name})
		c.focused = true
	}
	title := map[string]string{"add-gateway": "新建网关", "add-simulation": "新建模拟模型", "new-model": "新建并引用模型"}[c.action]
	items := []layout.FlexChild{layout.Rigid(e.th.bold(title, titleSize, colInk).Layout), vgap(16), layout.Rigid(e.th.label("名称", textSize, colBody).Layout), vgap(6), layout.Rigid(func(gtx C) D { return e.searchField(gtx, &c.name, "输入唯一名称") }), vgap(12)}
	if c.action == "add-gateway" {
		items = append(items, layout.Rigid(e.th.label("创建后分别添加上游和下游；配置完整并校验通过后可保存。", smallSize, colMuted).Layout))
	} else {
		items = append(items, layout.Rigid(e.th.label("持久化", textSize, colBody).Layout), vgap(6), layout.Rigid(func(gtx C) D {
			options := []layout.FlexChild{}
			for _, v := range []string{"memory", "file", "mmap", "sql"} {
				options = append(options, layout.Rigid(func(gtx C) D { return e.th.tab(gtx, c.clicks.get(v), v, c.persistence == v) }), gap(6))
			}
			return row(gtx, 34, options...)
		}))
		if c.persistence != "memory" {
			items = append(items, vgap(8), layout.Rigid(func(gtx C) D { return e.searchField(gtx, &c.location, "路径或 SQL 连接串") }))
		}
		if c.action == "new-model" {
			items = append(items, vgap(8), layout.Rigid(e.th.label("确认时同时创建模型并绑定当前下游；撤销同时恢复两者。", smallSize, colMuted).Layout))
		}
	}
	if c.problem != "" {
		items = append(items, vgap(8), layout.Rigid(e.th.label(c.problem, smallSize, colErr).Layout))
	}
	items = append(items, vgap(20), layout.Rigid(func(gtx C) D {
		return row(gtx, 34, layout.Rigid(func(gtx C) D { return e.th.button(gtx, c.clicks.get("cancel"), "取消", btnDefault) }), gap(8), layout.Rigid(func(gtx C) D { return e.th.button(gtx, c.clicks.get("confirm"), "创建", btnPrimary) }))
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
}
