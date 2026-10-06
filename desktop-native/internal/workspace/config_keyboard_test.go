package workspace

import (
	gioinput "gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
	"testing"
	"time"
)

func TestConfigurationKeyboardSaveSearchAndModalCancel(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	saved := ""
	u := New(Info{Config: parsed(t, text), Content: text, Running: true, Save: func(s string) error { saved = s; return nil }})
	s, e := u.view.shell, u.view.shell.cfg
	s.module = 1
	var router gioinput.Router
	frame := func() {
		var ops op.Ops
		u.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))})
		router.Frame(&ops)
	}
	frame()
	router.Queue(key.Event{Name: "F", Modifiers: key.ModCommand, State: key.Press})
	frame()
	frame()
	if !router.Source().Focused(&e.wb.query) {
		t.Fatal("Cmd+F did not focus search")
	}
	e.navigate("gateways.0")
	frame()
	e.eds["gateways.0.name"].SetText("keyboard-saved")
	frame()
	router.Queue(key.Event{Name: "S", Modifiers: key.ModCtrl, State: key.Press})
	frame()
	frame()
	if saved == "" || e.unsaved() {
		t.Fatal("Ctrl+S did not save draft")
	}
	beforeHistory := len(e.history)
	ed := e.eds["gateways.0.name"]
	router.Source().Execute(key.FocusCmd{Tag: ed})
	frame()
	ed.SetCaret(0, 0)
	router.Queue(key.EditEvent{Text: "typed-"})
	frame()
	router.Queue(key.Event{Name: "Z", Modifiers: key.ModCommand, State: key.Press})
	frame()
	if len(e.history) != beforeHistory {
		t.Fatal("text undo also consumed structural history")
	}
	router.Source().Execute(key.FocusCmd{Tag: &e.wb.query})
	frame()
	e.structure.get("add-gateway").Click()
	frame()
	frame()
	if e.creation == nil || !router.Source().Focused(&e.creation.name) {
		t.Fatal("creation dialog did not own focus")
	}
	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	frame()
	frame()
	if !router.Source().Focused(e.creation.clicks.get("cancel")) {
		t.Fatal("modal Tab escaped dialog")
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	frame()
	if e.creation != nil || len(e.draft.Gateways) != 1 {
		t.Fatal("Esc failed to cancel without mutation")
	}
}
