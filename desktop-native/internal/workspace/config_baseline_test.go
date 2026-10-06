package workspace

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/configfile"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/config"
	"image"
	"strings"
	"testing"
	"time"
)

func TestSavedRunningComparisonIgnoresUnsavedDraft(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	e.runningYAML = e.savedYAML
	e.runningIDs = copyIDs(e.savedIDs)
	e.saveFile = func(string) error { return nil }
	e.beginEdit()
	e.draft.Gateways[0].Name = "saved-B"
	e.eds["gateways.0.name"].SetText("saved-B")
	if err := e.doSave(); err != nil {
		t.Fatal(err)
	}
	e.beginEdit()
	e.draft.Gateways[0].Name = "draft-C"
	e.eds["gateways.0.name"].SetText("draft-C")
	pending := e.pendingChanges()
	found := false
	for _, c := range pending {
		if c.label == "名称" {
			found = true
			if c.new != "saved-B" || c.old != "demo" {
				t.Fatalf("running comparison leaked draft: %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("saved change missing")
	}
	for _, c := range e.changes() {
		if c.label == "名称" && (c.old != "saved-B" || c.new != "draft-C") {
			t.Fatalf("draft comparison changed: %+v", c)
		}
	}
}

func TestCreateAndReferenceIsAtomicAndUndoable(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	path := "gateways.0.downstreams.0"
	before := e.downstream(path).SimulationRef
	e.openCreation("new-model", path)
	e.creation.name.SetText("bound-model")
	if len(e.draft.Simulations) != 1 || e.downstream(path).SimulationRef != before {
		t.Fatal("opening dialog mutated draft")
	}
	if err := e.confirmCreation(); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Simulations) != 2 || e.downstream(path).SimulationRef != "bound-model" || e.sel != path {
		t.Fatal("creation did not bind and return to downstream")
	}
	if err := e.undoHistory(); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Simulations) != 1 || e.downstream(path).SimulationRef != before {
		t.Fatal("undo did not restore both objects")
	}
	e.openCreation("add-gateway", "")
	e.creation.name.SetText("fresh")
	if err := e.confirmCreation(); err != nil {
		t.Fatal(err)
	}
	g := e.draft.Gateways[1]
	if g.Name != "fresh" || len(g.Upstreams) != 0 || len(g.Downstreams) != 0 {
		t.Fatal("gateway creation fabricated devices")
	}
}

func TestCreationValidationAndCancellationPreserveDraft(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	before := e.visualYAML()
	e.openCreation("add-simulation", "")
	e.creation.name.SetText("model")
	if err := e.confirmCreation(); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if e.visualYAML() != before {
		t.Fatal("failed creation changed draft")
	}
	e.creation.clicks.get("cancel").Click()
	var ops op.Ops
	e.updateCreation(layout.Context{Ops: &ops})
	if e.visualYAML() != before {
		t.Fatal("cancel changed draft")
	}
}

func TestCommentOnlySavedDifferenceDoesNotRequireRestart(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	e.runningYAML = e.savedYAML
	e.savedYAML += "# operator note\n"
	if e.needsApply() {
		t.Fatal("comment-only edit requires runtime restart")
	}
	if len(e.pendingChanges()) == 0 {
		t.Fatal("text change not reviewable")
	}
}

func TestCustomBaudAndInvalidBufferModeSwitch(t *testing.T) {
	e := structureEditor(t, strings.Replace(liveConfig("127.0.0.1:15020"), `type: tcp`, `type: rtu`, 1))
	path := "gateways.0.upstreams.0.serial.baud_rate"
	s := e.node("gateways.0.upstreams.0").specs[3]
	if s.options != nil || s.check("57600") != "" {
		t.Fatal("custom baud disallowed")
	}
	e.beginEdit()
	e.eds[path].SetText("invalid")
	e.modes[1].Click()
	wbFrame(e)
	if e.raw || e.eds[path].Text() != "invalid" || e.structureErr == "" {
		t.Fatal("switch silently discarded invalid numeric input")
	}
}

func TestApplySavedConfigurationRequiresConfirmationAndKeepsFileOnCancel(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	rt := &fakeRuntime{state: live.State{Phase: live.Running}}
	saved := strings.Replace(text, "name: demo", "name: saved", 1)
	u := New(Info{Config: parsed(t, saved), Content: saved, RunningContent: text, Running: true, Runtime: rt})
	s, e := u.view.shell, u.view.shell.cfg
	s.module = 1
	frame := func() {
		var ops op.Ops
		u.Layout(layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))})
	}
	frame()
	e.saveApply.Click()
	frame()
	frame()
	if !s.confirming || rt.restarts != 0 {
		t.Fatal("saved configuration did not request confirmation")
	}
	s.cancel.Click()
	frame()
	if s.confirming || e.savedYAML != saved || !e.needsApply() || rt.restarts != 0 {
		t.Fatal("cancel changed saved file or runtime")
	}
	e.saveApply.Click()
	frame()
	frame()
	s.confirm.Click()
	frame()
	if rt.restarts != 1 {
		t.Fatal("confirm did not apply saved configuration")
	}
	rt.state.Phase = live.Starting
	e.saveApply.Click()
	frame()
	frame()
	if s.confirming || rt.restarts != 1 {
		t.Fatal("duplicate apply while starting")
	}
}

func TestConflictRebasePreservesDraftAndRunningBaseline(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	e := structureEditor(t, text)
	e.runningYAML = text
	disk := strings.Replace(text, "name: demo", "name: external", 1)
	draft := strings.Replace(text, "name: demo", "name: draft", 1)
	e.conflict = &configfile.Conflict{Baseline: text, Disk: disk, Draft: draft}
	e.rebaseFile = func(reviewed string) error {
		if reviewed != disk {
			t.Fatal("rebase accepted wrong version")
		}
		return nil
	}
	e.wb.clicks.get("conflict-rebase").Click()
	var ops op.Ops
	e.updateConflict(layout.Context{Ops: &ops})
	if e.conflict != nil || !e.raw || e.rawEd.Text() != draft || e.savedYAML != disk || e.runningYAML != text {
		t.Fatal("rebase lost one of three versions")
	}
	if !e.unsaved() {
		t.Fatal("merged draft appears saved")
	}
}

func TestProtocolSwitchKeepsEditedInactiveParameters(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	path := "gateways.0.downstreams.2"
	e.navigate(path)
	wbFrame(e)
	e.opts[path+".type"] = clicks[string]{}
	e.opts[path+".type"].get("rtu").Click()
	wbFrame(e)
	e.eds[path+".serial.device"].SetText("/dev/tty-custom")
	e.eds[path+".serial.baud_rate"].SetText("57600")
	wbFrame(e)
	e.opts[path+".type"].get("tcp").Click()
	wbFrame(e)
	if e.draft.Gateways[0].Downstreams[2].Type != "tcp" {
		t.Fatal("switch failed")
	}
	text := e.visualYAML()
	parsedCfg, err := config.ParseDraft([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	serial := parsedCfg.Gateways[0].Downstreams[2].Serial
	if serial.Device != "/dev/tty-custom" || serial.BaudRate != 57600 {
		t.Fatalf("edited inactive serial lost: %+v", serial)
	}
}

func TestCLISupportedUnnamedObjectsCanSave(t *testing.T) {
	text := strings.ReplaceAll(liveConfig("127.0.0.1:15020"), "name: demo", "name: ''")
	text = strings.ReplaceAll(text, "name: device", "name: ''")
	e := structureEditor(t, text)
	e.beginEdit()
	e.draft.Log.Level = "debug"
	e.version++
	if err := e.doSave(); err != nil {
		t.Fatalf("CLI-supported display labels rejected: %v", err)
	}
}

func TestModeSwitchDoesNotCreateDefaultFieldEdits(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	e := structureEditor(t, text)
	e.modes[1].Click()
	wbFrame(e)
	if !e.raw || e.rawEd.Text() != text || e.unsaved() {
		t.Fatal("view-only mode switch created an edit")
	}
	e.modes[0].Click()
	wbFrame(e)
	e.beginEdit()
	e.draft.Gateways[0].Name = "changed"
	e.eds["gateways.0.name"].SetText("changed")
	e.version++
	if out := e.visualYAML(); strings.Contains(out, "pprof:") || strings.Contains(out, "ui:") || strings.Contains(out, "log:") {
		t.Fatal("unmodified absent defaults were serialized")
	}
}
