package main

import (
	"gioui.org/widget"
	"github.com/ffutop/modbus-gateway/internal/config"
	"strings"
	"testing"
	"time"
)

func reviewEditor(t *testing.T, raw string) *configEditor {
	t.Helper()
	cfg, err := config.ParseDraft([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	e := &configEditor{draft: cfg, eds: map[string]*widget.Editor{}, savedYAML: raw, runningYAML: raw, rawSynced: raw}
	e.rebuild()
	e.commitBaseline()
	return e
}
func TestVisualPreservesAdvancedYAML(t *testing.T) {
	raw := `version: 1
# deployment comment
pprof: {enabled: true, address: "127.0.0.1:6060"}
gateways:
  - name: serial
    upstreams:
      - type: rtu
        serial: {device: /dev/ttyUSB0, baud_rate: 19200, data_bits: 8, parity: E, stop_bits: 1, timeout: 2s}
    downstreams:
      - name: meter
        type: tcp
        slave_ids: "1"
        tcp: {address: "localhost:502"}
`
	e := reviewEditor(t, raw)
	e.draft.Gateways[0].Name = "renamed"
	out := e.visualYAML()
	cfg, err := config.ParseDraft([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Pprof.Enabled || cfg.Gateways[0].Upstreams[0].Serial.Parity != "E" || !strings.Contains(out, "deployment comment") {
		t.Fatal(out)
	}
	if cfg.Gateways[0].Name != "renamed" {
		t.Fatal("edit missing")
	}
}
func TestSaveRetainsPendingApply(t *testing.T) {
	e := reviewEditor(t, "version: 1\ngateways: []\nlog: {level: info}\n")
	e.draft.Log.Level = "debug"
	e.doSave()
	if len(e.changes()) != 0 {
		t.Fatal("save did not clear draft changes")
	}
	if len(e.pendingChanges()) == 0 {
		t.Fatal("save lost pending apply differences")
	}
	e.runningYAML = e.savedYAML
	if len(e.pendingChanges()) != 0 {
		t.Fatal("applied changes remain pending")
	}
}
func TestV0PreservesRawVersion(t *testing.T) {
	raw := "# legacy\ngateways: []\nlog: {level: info}\n"
	e := reviewEditor(t, raw)
	e.draft.Log.Level = "debug"
	if e.visualYAML() != raw {
		t.Fatal("visual editor normalized legacy file")
	}
}

func TestApplyFailureRetainsRunningAndRetry(t *testing.T) {
	e := reviewEditor(t, "version: 1\ngateways: []\nlog: {level: info}\n")
	running := e.runningYAML
	e.draft.Log.Level = "debug"
	e.doSave()
	e.simulateFailure = true
	e.finishApply()
	if !e.applyFailed || e.runningYAML != running || len(e.pendingChanges()) == 0 {
		t.Fatal("failure lost running baseline or pending changes")
	}
	e.simulateFailure = false
	e.finishApply()
	if e.applyFailed || e.runningYAML != e.savedYAML || len(e.pendingChanges()) != 0 {
		t.Fatal("retry did not apply saved configuration")
	}
}
func TestRawInvalidNumberHasLineDiagnostic(t *testing.T) {
	e := reviewEditor(t, "version: 1\ngateways: []\n")
	e.rawEd.SetText("version: 1\ngateways:\n - name: test\n   upstreams:\n    - type: rtu\n      serial:\n       baud_rate: fast\n")
	e.reparse()
	line, msg := e.parseDiagnostic()
	if e.rawErr == "" || line != 7 || !strings.Contains(msg, "fast") || !strings.Contains(msg, "整数") {
		t.Fatalf("line=%d %s", line, msg)
	}
}
func TestSourceTableSelectAlignsTarget(t *testing.T) {
	e := reviewEditor(t, `version: 1
simulations:
 - name: model
   persistence: {type: memory}
gateways:
 - name: gw
   downstreams:
    - name: injector
      type: injector
      slave_ids: "1"
      simulation:
       ref: model
       mappings:
        - source: {table: holding_registers, start_address: 0, count: 1}
          target: {table: input_registers, start_address: 0}
`)
	e.alignMapping("gateways.0.downstreams.0.simulation.mappings.0.source.table", "coils")
	if e.draft.Gateways[0].Downstreams[0].Mappings[0].Target.Table != "discrete_inputs" {
		t.Fatal("target did not follow source table")
	}
}

func TestApplyingWaitsForRuntimeCompletion(t *testing.T) {
	e := newConfigEditor(NewTheme(), newWorld())
	old := e.runningYAML
	e.draft.Log.Level = "debug"
	e.doSave()
	e.finishApply()
	if !e.applying || e.runningYAML != old {
		t.Fatal("restart announced success before completion")
	}
	e.completeApply(e.applyUntil.Add(-time.Millisecond))
	if !e.applying || e.runningYAML != old {
		t.Fatal("configuration applied before restart completed")
	}
	e.completeApply(e.applyUntil)
	if e.applying || e.runningYAML != e.savedYAML {
		t.Fatal("restart completion did not apply saved revision")
	}
}
