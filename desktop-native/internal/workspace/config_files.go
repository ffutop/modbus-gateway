package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/filepicker"
)

// fileDialog picks a configuration file to open or to save the draft as when
// the platform has no file chooser (see package filepicker). It is drawn in
// the window, like the other dialogs.
type fileDialog struct {
	saveAs    bool
	dir       string
	entries   []os.DirEntry // directories first, then YAML files
	listErr   string
	location  widget.Editor // the directory shown; Enter goes there
	name      widget.Editor // file name, or a path; Enter confirms
	list      widget.List
	clicks    clicks[string]
	overwrite string // the existing path the next confirmation replaces
	problem   string
	focused   bool
}

func (e *configEditor) openFileDialog(saveAs bool) {
	d := &fileDialog{saveAs: saveAs, clicks: clicks[string]{}}
	d.location.SingleLine, d.location.Submit = true, true
	d.name.SingleLine, d.name.Submit = true, true
	d.list.Axis = layout.Vertical
	dir := filepath.Dir(e.configPath)
	if e.configPath == "" {
		dir, _ = os.Getwd()
	}
	if saveAs {
		d.name.SetText(filepath.Base(e.configPath))
	}
	d.cd(dir)
	e.files = d
}

// cd shows dir, keeping the current one if dir cannot be read.
func (d *fileDialog) cd(dir string) {
	dir = filepath.Clean(dir)
	all, err := os.ReadDir(dir)
	if err != nil {
		d.problem = "无法打开目录：" + err.Error()
		d.location.SetText(d.dir)
		return
	}
	d.dir, d.problem, d.overwrite, d.listErr = dir, "", "", ""
	d.location.SetText(dir)
	d.entries = d.entries[:0]
	for _, entry := range all {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if isDir(dir, entry) || yamlName(name) {
			d.entries = append(d.entries, entry)
		}
	}
	sort.SliceStable(d.entries, func(i, j int) bool {
		a, b := isDir(dir, d.entries[i]), isDir(dir, d.entries[j])
		if a != b {
			return a
		}
		return strings.ToLower(d.entries[i].Name()) < strings.ToLower(d.entries[j].Name())
	})
	d.list.Position = layout.Position{}
}

// isDir follows symlinks, so a linked directory can be entered.
func isDir(dir string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, entry.Name()))
	return err == nil && info.IsDir()
}

func yamlName(name string) bool { return filepicker.IsYAML(name) }

// target is the path the name field names, relative to the shown directory.
func (d *fileDialog) target() string {
	name := strings.TrimSpace(d.name.Text())
	if name == "" {
		return ""
	}
	if !filepath.IsAbs(name) {
		name = filepath.Join(d.dir, name)
	}
	if d.saveAs && filepath.Ext(name) == "" {
		name += ".yaml"
	}
	return filepath.Clean(name)
}

func (e *configEditor) confirmFile() error {
	d := e.files
	path := d.target()
	if path == "" {
		return fmt.Errorf("请选择或输入文件名")
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		d.name.SetText("")
		d.cd(path)
		return nil
	}
	if !d.saveAs {
		if e.unsaved() {
			return fmt.Errorf("当前配置有未保存的修改，请先保存、撤销，或另存为")
		}
		if e.openFile == nil {
			return fmt.Errorf("无法切换配置文件")
		}
		if err := e.openFile(path); err != nil {
			return err
		}
		e.files = nil
		return nil
	}
	if _, err := os.Stat(path); err == nil && d.overwrite != path {
		d.overwrite = path
		return nil
	}
	text, err := e.draftText()
	if err != nil {
		return err
	}
	if e.saveAsFile == nil {
		return fmt.Errorf("无法另存配置文件")
	}
	if err := e.saveAsFile(path, text); err != nil {
		return err
	}
	e.files = nil
	return nil
}

func (e *configEditor) updateFiles(gtx C) {
	d := e.files
	if d == nil {
		return
	}
	for {
		ev, ok := d.location.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			path := strings.TrimSpace(d.location.Text())
			if path != "" && !filepath.IsAbs(path) {
				path = filepath.Join(d.dir, path)
			}
			d.cd(path)
		}
	}
	submit := false
	for {
		ev, ok := d.name.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.SubmitEvent:
			submit = true
		case widget.ChangeEvent:
			d.overwrite, d.problem = "", ""
		}
	}
	for _, entry := range d.entries {
		btn := d.clicks.get("entry|" + entry.Name())
		for {
			click, ok := btn.Update(gtx)
			if !ok {
				break
			}
			if isDir(d.dir, entry) {
				d.cd(filepath.Join(d.dir, entry.Name()))
				return // the entries changed
			}
			d.name.SetText(entry.Name())
			d.overwrite, d.problem = "", ""
			submit = submit || click.NumClicks > 1
		}
	}
	if d.clicks.get("up").Clicked(gtx) {
		d.cd(filepath.Dir(d.dir))
	}
	if d.clicks.get("cancel").Clicked(gtx) {
		e.closeFiles(gtx)
		return
	}
	if d.clicks.get("confirm").Clicked(gtx) || submit {
		if err := e.confirmFile(); err != nil {
			d.problem = err.Error()
		}
	}
}

func (e *configEditor) closeFiles(gtx C) {
	btn := &e.openBtn
	if e.files.saveAs {
		btn = &e.saveAsBtn
	}
	e.files = nil
	gtx.Execute(key.FocusCmd{Tag: btn})
}

func (e *configEditor) filesPane(gtx C) D {
	d := e.files
	th := e.th
	if !d.focused {
		gtx.Execute(key.FocusCmd{Tag: &d.name})
		d.focused = true
	}
	title, action := "打开配置文件", "打开"
	if d.saveAs {
		title, action = "另存为", "保存"
		if d.overwrite != "" {
			action = "覆盖保存"
		}
	}
	hint := "显示目录和 .yaml / .yml 文件。单击文件选中，双击或回车打开。"
	if d.saveAs {
		hint = "保存当前草稿（需校验通过）并切换到新文件；未写扩展名时补 .yaml。运行中的网关在重启后使用新文件。"
	} else if e.unsaved() {
		hint = "当前配置有未保存的修改；请先保存、撤销，或改用「另存为」。"
	}
	items := []layout.FlexChild{
		layout.Rigid(th.bold(title, titleSize, colInk).Layout), vgap(12),
		layout.Rigid(func(gtx C) D {
			return row(gtx, 34,
				layout.Rigid(func(gtx C) D { return th.button(gtx, d.clicks.get("up"), "上一级", btnDefault) }), gap(8),
				layout.Flexed(1, func(gtx C) D { return e.searchField(gtx, &d.location, "目录，回车前往") }))
		}), vgap(8),
		layout.Flexed(1, func(gtx C) D {
			return outlined(gtx, colLine, colCanvas, radiusSm, func(gtx C) D {
				gtx.Constraints.Min = gtx.Constraints.Max
				if len(d.entries) == 0 {
					return layout.UniformInset(12).Layout(gtx, th.label("此目录没有子目录或 YAML 文件。", smallSize, colMuted).Layout)
				}
				return material.List(th.Theme, &d.list).Layout(gtx, len(d.entries), func(gtx C, i int) D {
					entry := d.entries[i]
					dir := isDir(d.dir, entry)
					btn := d.clicks.get("entry|" + entry.Name())
					return btn.Layout(gtx, func(gtx C) D {
						bg := colCanvas
						if !dir && entry.Name() == strings.TrimSpace(d.name.Text()) {
							bg = colSelected
						} else if btn.Hovered() {
							bg = colHover
						}
						return background(gtx, bg, func(gtx C) D {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							label, col := entry.Name(), colBody
							if dir {
								label, col = entry.Name()+string(filepath.Separator), colInk
							}
							l := th.label(label, textSize, col)
							l.MaxLines = 1
							return layout.Inset{Left: 10, Right: 10, Top: 6, Bottom: 6}.Layout(gtx, l.Layout)
						})
					})
				})
			})
		}), vgap(8),
		layout.Rigid(func(gtx C) D {
			return row(gtx, 34,
				layout.Rigid(th.label("文件名", textSize, colBody).Layout), gap(8),
				layout.Flexed(1, func(gtx C) D {
					hint := "选择文件，或输入文件名／路径"
					if d.saveAs {
						hint = "config.yaml"
					}
					return e.searchField(gtx, &d.name, hint)
				}))
		}), vgap(8),
		layout.Rigid(func(gtx C) D { return th.label(hint, smallSize, colMuted).Layout(gtx) }),
	}
	if d.overwrite != "" {
		items = append(items, vgap(6), layout.Rigid(th.label(d.overwrite+" 已存在；再次确认将覆盖它。", smallSize, colWarn).Layout))
	}
	if d.problem != "" {
		items = append(items, vgap(6), layout.Rigid(th.label(d.problem, smallSize, colErr).Layout))
	}
	items = append(items, vgap(16), layout.Rigid(func(gtx C) D {
		return row(gtx, 34,
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx C) D { return th.button(gtx, d.clicks.get("cancel"), "取消", btnDefault) }), gap(8),
			layout.Rigid(func(gtx C) D {
				kind := btnPrimary
				if d.overwrite != "" {
					kind = btnDanger
				}
				draw := func(gtx C) D { return th.button(gtx, d.clicks.get("confirm"), action, kind) }
				if !d.saveAs && e.unsaved() {
					return disabled(gtx, draw)
				}
				return draw(gtx)
			}))
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
}

// filePick is a system file chooser in progress. It runs on its own
// goroutine; the window polls for its result while input stays blocked.
type filePick struct {
	saveAs bool
	text   string // the validated draft to write when saving
	done   chan pickResult
}

type pickResult struct {
	path string
	err  error
}

// chooseFile asks for a file to open or to save the draft as, with the
// platform's chooser when there is one and the in-app dialog otherwise.
func (e *configEditor) chooseFile(gtx C, saveAs bool) {
	var text string
	if saveAs {
		var err error
		if text, err = e.draftText(); err != nil {
			e.notify(gtx, "无法另存："+err.Error(), true)
			return
		}
	} else if e.unsaved() {
		e.notify(gtx, "当前配置有未保存的修改，请先保存、撤销，或另存为。", true)
		return
	}
	if e.pickFile == nil {
		e.openFileDialog(saveAs)
		return
	}
	p := &filePick{saveAs: saveAs, text: text, done: make(chan pickResult, 1)}
	e.picking = p
	pick, dir, name := e.pickFile, filepath.Dir(e.configPath), filepath.Base(e.configPath)
	go func() {
		path, err := pick(saveAs, dir, name)
		p.done <- pickResult{path, err}
	}()
}

func (e *configEditor) updatePick(gtx C) {
	p := e.picking
	if p == nil {
		return
	}
	select {
	case r := <-p.done:
		e.picking = nil
		e.finishPick(gtx, p, r)
	default:
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(100 * time.Millisecond)})
	}
}

func (e *configEditor) finishPick(gtx C, p *filePick, r pickResult) {
	switch {
	case errors.Is(r.err, filepicker.ErrUnavailable):
		e.openFileDialog(p.saveAs)
	case r.err != nil:
		e.notify(gtx, r.err.Error(), true)
	case r.path == "":
		// cancelled
	case p.saveAs:
		path := r.path
		if filepath.Ext(path) == "" {
			// The chooser confirmed replacing the name without extension only.
			path += ".yaml"
			if _, err := os.Stat(path); err == nil {
				e.notify(gtx, path+" 已存在；如要覆盖，请在保存对话框中直接选择该文件。", true)
				return
			}
		}
		if err := e.saveAsFile(path, p.text); err != nil {
			e.notify(gtx, "另存失败："+err.Error(), true)
		}
	default:
		if err := e.openFile(r.path); err != nil {
			e.notify(gtx, "打开失败："+err.Error(), true)
		}
	}
}

func (e *configEditor) notify(gtx C, msg string, failed bool) {
	e.toast, e.toastAt, e.toastErr = msg, gtx.Now, failed
}
