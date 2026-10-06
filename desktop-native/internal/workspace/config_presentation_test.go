package workspace

import (
	"gioui.org/f32"
	gioinput "gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
	"testing"
	"time"
)

// Exercise the rendered selectors through Gio's input router. This catches
// popups that look correct but are clipped, covered, or closed on the next frame.
func TestConfigurationSelectorsNavigateEditAndSave(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	saved := ""
	u := New(Info{Config: parsed(t, text), Content: text, Running: true, Save: func(s string) error { saved = s; return nil }})
	s, e := u.view.shell, u.view.shell.cfg
	s.module = 1
	e.sel = "group:网关"
	var router gioinput.Router
	frame := func() {
		var ops op.Ops
		u.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))})
		router.Frame(&ops)
	}
	click := func(x, y float32) {
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, y)})
		frame()
		frame()
	}
	frame()
	click(390, 163)
	if !e.wb.filterOpen {
		t.Fatal("status selector did not remain open")
	}
	click(390, 230)
	if e.wb.filterOpen || e.wb.filter != "有问题" {
		t.Fatalf("status choice did not apply: %q open=%v", e.wb.filter, e.wb.filterOpen)
	}
	e.wb.filter = "全部"
	e.navigate("gateways.0.downstreams.1")
	frame()
	click(850, 385)
	if e.wb.optionPath != "gateways.0.downstreams.1.type" {
		t.Fatal("protocol selector did not open")
	}
	click(850, 480)
	if e.downstream(e.sel).Type != "tcp" || e.wb.optionPath != "" {
		t.Fatal("protocol choice was not committed")
	}
	e.eds["gateways.0.downstreams.1.tcp.address"].SetText("127.0.0.1:16021")
	frame()
	router.Queue(key.Event{Name: "S", Modifiers: key.ModCtrl, State: key.Press})
	frame()
	frame()
	if saved == "" || e.unsaved() {
		t.Fatal("edited selector configuration did not save")
	}
	cfg := parsed(t, saved)
	if cfg.Gateways[0].Downstreams[1].Type != "tcp" || cfg.Gateways[0].Downstreams[1].Tcp.Address != "127.0.0.1:16021" {
		t.Fatal("saved file lost edited protocol or address")
	}
}
