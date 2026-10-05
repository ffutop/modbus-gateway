package workspace

import (
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/gateway"
)

// fakeRuntime records restarts and reports a settable state.
type fakeRuntime struct {
	state    live.State
	restarts int
}

func (f *fakeRuntime) State() live.State { return f.state }
func (f *fakeRuntime) Restart()          { f.restarts++ }

// fakeSource reports fixed listener states.
type fakeSource struct {
	live.Local
	ups []gateway.UpstreamStatus
}

func (f fakeSource) Upstreams() []gateway.UpstreamStatus { return f.ups }

func TestStatusBarFollowsTheGatewayProcessAndListeners(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	rt := &fakeRuntime{}
	src := &fakeSource{}
	u := New(Info{Config: parsed(t, text), Content: text, Running: true, Runtime: rt, Source: src})
	s := u.view.shell
	for _, c := range []struct {
		state live.State
		ups   []gateway.UpstreamStatus
		want  string
		err   bool
	}{
		{live.State{Phase: live.Starting}, nil, "正在启动网关", false},
		{live.State{Phase: live.Running}, nil, "正在获取监听状态", false},
		{live.State{Phase: live.Running}, []gateway.UpstreamStatus{{State: gateway.UpstreamListening}, {State: gateway.UpstreamStarting}}, "监听 1/2，其余启动中", false},
		{live.State{Phase: live.Running}, []gateway.UpstreamStatus{{State: gateway.UpstreamListening}}, "运行中 · 监听 1/1", false},
		{live.State{Phase: live.Running}, []gateway.UpstreamStatus{{State: gateway.UpstreamListening}, {Gateway: "demo", Index: 1, State: gateway.UpstreamFailed, Error: "address in use"}}, "监听失败 1/2 · demo 上游 2：address in use", true},
		{live.State{Phase: live.Stopped, Err: errors.New("网关意外退出（signal: killed）\nlast log line")}, nil, "网关未运行：网关意外退出（signal: killed）", true},
	} {
		rt.state, src.ups = c.state, c.ups
		got, col := s.runtimeText()
		if !strings.Contains(got, c.want) || strings.Contains(got, "\n") || (col == colErr) != c.err {
			t.Errorf("state %+v ups %+v: got %q (error color %v), want %q (error color %v)", c.state, c.ups, got, col == colErr, c.want, c.err)
		}
	}
}

func TestRestartNeedsSavedConfigAndWarnsAboutMemoryModels(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	rt := &fakeRuntime{state: live.State{Phase: live.Running}}
	u := New(Info{Config: parsed(t, text), Content: text, Running: true, Runtime: rt})
	s := u.view.shell
	if why := s.canRestart(); why != "" {
		t.Fatalf("clean running gateway cannot restart: %s", why)
	}
	if w := s.restartWarning(); !strings.Contains(w, "中断全部链路") || !strings.Contains(w, "memory 模型的数据将清空") {
		t.Fatalf("warning = %q", w)
	}
	s.cfg.draft.Gateways[0].Name = "changed"
	s.cfg.version++
	if why := s.canRestart(); !strings.Contains(why, "保存") {
		t.Fatalf("restart allowed with unsaved edits: %q", why)
	}
	s.cfg.draft.Gateways[0].Name = "demo"
	rt.state.Phase = live.Starting
	if why := s.canRestart(); why == "" {
		t.Fatal("restart allowed while starting")
	}
	if rt.restarts != 0 {
		t.Fatal("restart without confirmation")
	}
}

func TestRestartAsksForConfirmationFirst(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	rt := &fakeRuntime{state: live.State{Phase: live.Running}}
	u := New(Info{Config: parsed(t, text), Content: text, Running: true, Runtime: rt})
	frame := func() {
		var ops op.Ops
		u.Layout(layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1440, 900))})
	}
	s := u.view.shell
	frame()
	s.restart.Click()
	frame()
	if !s.confirming || rt.restarts != 0 {
		t.Fatalf("first click: confirming = %v, restarts = %d; want the warning only", s.confirming, rt.restarts)
	}
	s.cancel.Click()
	frame()
	if s.confirming || rt.restarts != 0 {
		t.Fatal("cancel did not dismiss the warning")
	}
	s.restart.Click()
	frame()
	s.confirm.Click()
	frame()
	if s.confirming || rt.restarts != 1 {
		t.Fatalf("confirm: confirming = %v, restarts = %d; want one restart", s.confirming, rt.restarts)
	}
}

func TestDeleteDialogKeepsReferencePickerAndCancelInModal(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	u := New(Info{Config: parsed(t, text), Content: text, Running: true})
	s, e := u.view.shell, u.view.shell.cfg
	s.module = 1
	e.navigate("simulations.0")
	frame := func() {
		var ops op.Ops
		u.Layout(layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))})
	}
	frame()
	before := e.recoveryContent()
	e.structure.get("request-delete|simulations.0").Click()
	frame()
	if !s.modalOpen() {
		t.Fatal("delete did not open modal")
	}
	if e.recoveryContent() != before {
		t.Fatal("opening dialog changed draft")
	}
	e.wb.clicks.get("picker|replace-all").Click()
	frame()
	if !s.modalOpen() || e.wb.picker == "" {
		t.Fatal("reference picker escaped deletion modal")
	}
	e.structure.get("cancel-delete|").Click()
	frame()
	if s.modalOpen() || e.wb.picker != "" {
		t.Fatal("cancel left a modal or picker open")
	}
	if e.recoveryContent() != before {
		t.Fatal("cancel changed draft")
	}
}
