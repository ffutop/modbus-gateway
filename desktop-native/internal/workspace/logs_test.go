package workspace

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gioinput "gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/runlog"
)

func TestLogsFilterPauseClearAndRebuild(t *testing.T) {
	s := runlog.New(nil)
	s.Record("gateway", 1, "ERROR", "Connection failed", "gateway", "line-a")
	s.Record("desktop", 0, "INFO", "Saved configuration")
	v := newLogView(NewTheme(), s, nil)
	now := time.Now()
	v.refresh(now)
	if len(v.shown) != 2 {
		t.Fatal("missing logs")
	}
	v.paused = true
	s.Record("gateway", 2, "WARN", "Retrying", "gateway", "line-b")
	v.refresh(now.Add(time.Second))
	if len(v.shown) != 2 {
		t.Fatal("paused snapshot changed")
	}
	v.paused = false
	v.refresh(now.Add(2 * time.Second))
	if len(v.shown) != 3 {
		t.Fatal("resume missed new records")
	}
	v.level = 1
	v.gateway.SetText("LINE-A")
	v.query.SetText("connection")
	v.refresh(now.Add(3 * time.Second))
	if len(v.shown) != 1 || !strings.Contains(v.text(), "failed") {
		t.Fatal("filter mismatch")
	}
	v.selectEntry(v.shown[0])
	if !strings.Contains(v.detail.Text(), "line-a") || v.follow {
		t.Fatal("details/follow mismatch")
	}
	v.clear()
	v.level = 0
	v.gateway.SetText("")
	v.query.SetText("")
	v.refresh(now.Add(4 * time.Second))
	if len(v.shown) != 0 || len(s.Snapshot(0).Entries) != 3 {
		t.Fatal("clear removed source history or revived old rows")
	}
	s.Record("desktop", 0, "INFO", "new")
	v.refresh(now.Add(5 * time.Second))
	if len(v.shown) != 1 {
		t.Fatal("new record missing after clear")
	}
	text := liveConfig("127.0.0.1:15020")
	u := New(Info{Config: parsed(t, text), Content: text, Logs: s, Running: true})
	u.view.shell.module = 2
	u.view.shell.logs = v
	next := New(Info{Config: parsed(t, text), Content: text, Logs: s, Running: true})
	u.CarryDraftTo(next)
	if next.view.shell.module != 2 || next.view.shell.logs != v {
		t.Fatal("log view reset during restart")
	}
}

func TestLogTabActionsAndMinimumLayout(t *testing.T) {
	s := runlog.New(nil)
	s.Record("gateway", 1, "ERROR", strings.Repeat("长日志消息", 100), "gateway", "demo")
	text := liveConfig("127.0.0.1:15020")
	u := New(Info{Config: parsed(t, text), Content: text, Logs: s, Running: true})
	sh := u.view.shell
	sh.modules[2].Click()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))}
	d := u.Layout(gtx)
	if sh.module != 2 || d.Size != image.Pt(1100, 680) {
		t.Fatal("log tab not displayed")
	}
	sh.logs.selectEntry(sh.logs.shown[0])
	sh.logs.buttons.get("export").Click()
	ops.Reset()
	u.Layout(gtx)
	if !sh.logs.exporting || !strings.Contains(sh.logs.exportText, "长日志消息") {
		t.Fatal("export snapshot not captured")
	}
}

func TestLogSearchShortcutAndExportConfirmation(t *testing.T) {
	s := runlog.New(nil)
	s.Record("desktop", 0, "INFO", "first")
	text := liveConfig("127.0.0.1:15020")
	u := New(Info{Config: parsed(t, text), Content: text, Logs: s, Running: true})
	sh := u.view.shell
	sh.module = 2
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
	if !router.Source().Focused(&sh.logs.query) {
		t.Fatal("log search shortcut failed")
	}
	v := sh.logs
	v.buttons.get("export").Click()
	frame()
	path := filepath.Join(t.TempDir(), "output.log")
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	v.path.SetText(path)
	v.buttons.get("confirm-export").Click()
	frame()
	result := <-v.result
	v.result <- result
	frame()
	if v.overwrite != path || !v.exporting {
		t.Fatal("missing overwrite confirmation")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "existing" {
		t.Fatal("unconfirmed overwrite")
	}
	s.Record("desktop", 0, "INFO", "later")
	v.buttons.get("confirm-export").Click()
	frame()
	result = <-v.result
	v.result <- result
	frame()
	b, _ = os.ReadFile(path)
	if v.exporting || !strings.Contains(string(b), "first") || strings.Contains(string(b), "later") {
		t.Fatal("export did not preserve captured snapshot")
	}
}
