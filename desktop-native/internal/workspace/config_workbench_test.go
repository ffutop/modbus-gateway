package workspace

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/ffutop/modbus-gateway/internal/config"
	"image"
	"strings"
	"testing"
	"time"
)

func wbFrame(e *configEditor) {
	var ops op.Ops
	e.Layout(layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))})
}
func twoGatewayEditor(t *testing.T) *configEditor {
	return structureEditor(t, liveConfig("127.0.0.1:15020")+`  - name: second
    upstreams: [{type: tcp, tcp: {address: "127.0.0.1:15022"}}]
    downstreams: [{name: other, type: tcp, slave_ids: "4", tcp: {address: "127.0.0.1:15023"}}]
`)
}
func TestWorkbenchSearchFiltersAndBidirectionalNavigation(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	e.wb.query.SetText("15021")
	matches := 0
	for _, n := range e.nodes {
		if e.matches(n, e.pendingIDs()) {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("address search matched %d nodes", matches)
	}
	e.wb.query.SetText("holding_registers")
	if !e.matches(e.node("gateways.0.downstreams.1"), e.pendingIDs()) {
		t.Fatal("mapping not searchable")
	}
	e.navigate("simulations.0")
	e.navigate("gateways.0.downstreams.0")
	e.wb.clicks.get("back").Click()
	wbFrame(e)
	if e.sel != "simulations.0" {
		t.Fatal("back lost context")
	}
	command(t, e, "add-simulation", "", nil)
	e.wb.query.SetText("")
	e.wb.filter = "无引用"
	if !e.matches(e.node("simulations.1"), nil) || e.matches(e.node("simulations.0"), nil) {
		t.Fatal("unused filter wrong")
	}
}
func TestCompletedFieldsAndMappingsShareUndoRedoHistory(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	e.beginEdit()
	e.draft.Gateways[0].Downstreams[2].Tcp.Address = "127.0.0.1:16000"
	e.eds["gateways.0.downstreams.2.tcp.address"].SetText("127.0.0.1:16000")
	if err := e.finishEdits(); err != nil {
		t.Fatal(err)
	}
	if len(e.history) != 1 {
		t.Fatal("field edit not one transaction")
	}
	command(t, e, "rename", "simulations.0", map[string]string{"name": "renamed"})
	if err := e.undoHistory(); err != nil {
		t.Fatal(err)
	}
	if len(e.references("model")) != 2 || e.draft.Gateways[0].Downstreams[2].Tcp.Address != "127.0.0.1:16000" {
		t.Fatal("undo did not isolate transaction")
	}
	if err := e.undoHistory(); err != nil {
		t.Fatal(err)
	}
	if e.draft.Gateways[0].Downstreams[2].Tcp.Address != "127.0.0.1:15021" {
		t.Fatal("field undo failed")
	}
	if err := e.redoHistory(); err != nil {
		t.Fatal(err)
	}
	if err := e.redoHistory(); err != nil {
		t.Fatal(err)
	}
	if len(e.references("renamed")) != 2 {
		t.Fatal("redo references lost")
	}
}
func TestSerialAdvancedParametersRoundTrip(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	e.beginEdit()
	d := &e.draft.Gateways[0].Downstreams[2]
	d.Type = "rtu"
	d.Tcp.Address = ""
	d.Serial = config.SerialConfig{Device: "/dev/test", BaudRate: 19200, DataBits: 7, Parity: "E", StopBits: 2, Timeout: 2 * time.Second, RqstPause: 300 * time.Millisecond, RS485: true, DelayRtsBeforeSend: 10 * time.Millisecond, DelayRtsAfterSend: 20 * time.Millisecond, RtsHighDuringSend: true, RtsHighAfterSend: true, RxDuringTx: true}
	e.resetControls()
	e.rebuild()
	if err := e.finishEdits(); err != nil {
		t.Fatal(err)
	}
	got := parsed(t, e.visualYAML()).Gateways[0].Downstreams[2].Serial
	if got != d.Serial {
		t.Fatalf("advanced roundtrip: got %+v want %+v", got, d.Serial)
	}
}
func TestBatchCopyIndependentModelsAndUniqueSlaveSuggestions(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	e.wb.bulk = newBulk("copy", []string{e.node("gateways.0").id})
	e.wb.bulk.independent.Value = true
	if err := e.previewBulk(); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Gateways) != 1 {
		t.Fatal("preview modified draft")
	}
	if err := e.confirmBulk(); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Gateways) != 2 || len(e.draft.Simulations) != 2 {
		t.Fatal("independent copy incomplete")
	}
	copy := e.draft.Gateways[1]
	if copy.Upstreams[0].Tcp.Address != "" || copy.Downstreams[0].SimulationRef == "model" {
		t.Fatal("copy silently reused listener or model")
	}
	e.wb.bulk = newBulk("copy", []string{e.node("gateways.0.downstreams.0").id, e.node("gateways.0.downstreams.2").id})
	if err := e.previewBulk(); err != nil {
		t.Fatal(err)
	}
	if err := e.confirmBulk(); err != nil {
		t.Fatal(err)
	}
	ds := e.draft.Gateways[0].Downstreams
	if ds[3].SlaveIDs == ds[4].SlaveIDs {
		t.Fatal("copy generated duplicate IDs")
	}
}
func TestBatchMovePreservesIdentityAndRejectsStalePreview(t *testing.T) {
	e := twoGatewayEditor(t)
	id := e.node("gateways.0.downstreams.2").id
	e.wb.bulk = newBulk("move", []string{id})
	e.wb.bulk.field("gateway", "2")
	if err := e.previewBulk(); err != nil {
		t.Fatal(err)
	}
	e.wb.bulk.field("gateway", "").SetText("1")
	if err := e.confirmBulk(); err == nil {
		t.Fatal("stale field preview accepted")
	}
	e.wb.bulk.field("gateway", "").SetText("2")
	if err := e.previewBulk(); err != nil {
		t.Fatal(err)
	}
	if err := e.confirmBulk(); err != nil {
		t.Fatal(err)
	}
	if e.node("gateways.1.downstreams.1").id != id || len(e.draft.Gateways[0].Downstreams) != 2 {
		t.Fatal("move lost identity or source")
	}
	if err := e.undoHistory(); err != nil {
		t.Fatal(err)
	}
	if e.node("gateways.0.downstreams.2").id != id {
		t.Fatal("move undo identity lost")
	}
}
func TestBatchDeleteUsesFinalReferenceGraph(t *testing.T) {
	e := twoGatewayEditor(t)
	e.wb.bulk = newBulk("delete", []string{e.node("simulations.0").id, e.node("gateways.0").id})
	if err := e.previewBulk(); err != nil {
		t.Fatal(err)
	}
	if err := e.confirmBulk(); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Simulations) != 0 || len(e.draft.Gateways) != 1 || e.draft.Gateways[0].Name != "second" {
		t.Fatal("wrong final deletion graph")
	}
}
func TestBatchConflictDoesNotPartiallyModifyDraft(t *testing.T) {
	e := twoGatewayEditor(t)
	before := e.visualYAML()
	e.wb.bulk = newBulk("edit", []string{e.node("gateways.0.downstreams.0").id})
	e.wb.bulk.field("slave_start", "2")
	if err := e.previewBulk(); err == nil {
		t.Fatal("duplicate route accepted")
	}
	if e.visualYAML() != before {
		t.Fatal("failed batch mutated draft")
	}
}
func TestBatchAddPreviewAndUndo(t *testing.T) {
	e := twoGatewayEditor(t)
	e.wb.bulk = newBulk("add", nil)
	for k, v := range map[string]string{"gateway": "2", "count": "3", "slave_start": "20", "name_prefix": "sensor", "type": "tcp", "tcp.address": "127.0.0.1:16000"} {
		e.wb.bulk.field(k, v)
	}
	if err := e.previewBulk(); err != nil {
		t.Fatal(err)
	}
	if err := e.confirmBulk(); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Gateways[1].Downstreams) != 4 || e.draft.Gateways[1].Downstreams[3].SlaveIDs != "22" {
		t.Fatal("batch add wrong")
	}
	if err := e.undoHistory(); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Gateways[1].Downstreams) != 1 {
		t.Fatal("batch undo was partial")
	}
}
func TestRecoveryPreservesInvalidInputWithoutTouchingSavedConfig(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	saved := e.savedYAML
	e.eds["gateways.0.downstreams.1.simulation.mappings.0.source.count"].SetText("oops")
	text := e.recoveryContent()
	if !strings.Contains(text, "oops") || e.savedYAML != saved {
		t.Fatal("incomplete input lost or saved changed")
	}
	e.recoveryText = text
	if err := e.recoverDraft(); err != nil {
		t.Fatal(err)
	}
	if !e.raw || e.rawErr == "" || e.savedYAML != saved {
		t.Fatal("invalid recovery not isolated")
	}
	if err := e.undoHistory(); err != nil {
		t.Fatal(err)
	}
}
func TestGatewayWizardCanCompleteAndKeepIncompleteDraft(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	if err := e.startWizard(); err != nil {
		t.Fatal(err)
	}
	if err := e.wizardNext(); err != nil {
		t.Fatal(err)
	}
	path := e.sel
	e.eds[path+".tcp.address"].SetText("127.0.0.1:15024")
	wbFrame(e)
	if err := e.wizardNext(); err != nil {
		t.Fatal(err)
	}
	path = e.sel
	e.eds[path+".tcp.address"].SetText("127.0.0.1:15025")
	e.eds[path+".slave_ids"].SetText("1")
	wbFrame(e)
	if err := e.wizardNext(); err != nil {
		t.Fatal(err)
	}
	if e.wb.wizard != nil || len(e.draft.Gateways) != 2 {
		t.Fatal("wizard incomplete")
	}
}
func TestSharedModelSelectorStagesChoiceAndChecksInjectorConflict(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	command(t, e, "add-simulation", "", nil)
	name := e.draft.Simulations[1].Name
	e.wb.picker = "gateways.0.downstreams.0.simulation.ref"
	if err := e.chooseModel(name); err != nil {
		t.Fatal(err)
	}
	if e.editBase == nil || e.draft.Gateways[0].Downstreams[0].SimulationRef != name {
		t.Fatal("selection not staged")
	}
	if err := e.finishEdits(); err != nil {
		t.Fatal(err)
	}
	if err := e.undoHistory(); err != nil {
		t.Fatal(err)
	}
	if e.draft.Gateways[0].Downstreams[0].SimulationRef != "model" {
		t.Fatal("reference selection undo failed")
	}
}

func TestBulkPreviewRemainsValidWhenEmptyControlsInitialize(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	e.wb.bulk = newBulk("copy", []string{e.node("gateways.0").id})
	if err := e.previewBulk(); err != nil {
		t.Fatal(err)
	}
	revision := e.wb.bulk.requestRevision
	e.wb.bulk.field("name_prefix", "")
	if e.wb.bulk.fingerprint() != revision {
		t.Fatal("empty control invalidated preview")
	}
	e.wb.bulk.field("name_prefix", "").SetText("changed")
	if err := e.confirmBulk(); err == nil {
		t.Fatal("changed request accepted old preview")
	}
	if len(e.draft.Gateways) != 1 {
		t.Fatal("stale preview mutated draft")
	}
}
