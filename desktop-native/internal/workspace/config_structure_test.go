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

func structureEditor(t *testing.T, text string) *configEditor {
	t.Helper()
	return newConfigEditor(NewTheme(), Info{Config: parsed(t, text), Content: text, Running: true, Save: func(string) error { return nil }})
}
func command(t *testing.T, e *configEditor, action, path string, replacements map[string]string) {
	t.Helper()
	if err := e.mutateStructure(action, path, replacements); err != nil {
		t.Fatal(err)
	}
}
func TestStructureDeletionKeepsIdentityAndSavedRuntimeDiff(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	survivor := e.node("gateways.0.downstreams.1").id
	command(t, e, "delete", "gateways.0.downstreams.0", nil)
	if e.node("gateways.0.downstreams.0").id != survivor {
		t.Fatal("survivor lost identity")
	}
	changes := e.changes()
	if len(changes) != 1 || !changes[0].removed || changes[0].old != "local" {
		t.Fatalf("false changes: %+v", changes)
	}
	if err := e.doSave(); err != nil {
		t.Fatal(err)
	}
	pending := e.pendingChanges()
	if len(pending) != 1 || !pending[0].removed || pending[0].old != "local" {
		t.Fatalf("pending diff: %+v", pending)
	}
	cfg := parsed(t, e.savedYAML)
	if len(cfg.Gateways[0].Downstreams) != 2 || cfg.Gateways[0].Downstreams[0].Name != "injector" {
		t.Fatal("wrong serialized structure")
	}
	if !strings.Contains(e.runningYAML, "name: local") {
		t.Fatal("save changed running configuration")
	}
}
func TestStructureUndoRestoresReferencesIdentityAndSelection(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	e.sel = "simulations.0"
	id := e.node(e.sel).id
	command(t, e, "rename", e.sel, map[string]string{"name": "renamed"})
	if len(e.references("renamed")) != 2 || len(e.references("model")) != 0 {
		t.Fatal("references not updated")
	}
	if err := e.restore(e.undoState); err != nil {
		t.Fatal(err)
	}
	if e.sel != "simulations.0" || e.node(e.sel).id != id || len(e.references("model")) != 2 || len(e.changes()) != 0 {
		t.Fatal("undo did not restore editor and references")
	}
}
func TestModelMigrationAcrossGatewaysIsAtomic(t *testing.T) {
	text := liveConfig("127.0.0.1:15020") + `  - name: second
    upstreams:
      - type: tcp
        tcp: {address: "127.0.0.1:15022"}
    downstreams:
      - name: shared
        type: local
        slave_ids: "3"
        simulation: {ref: model}
`
	e := structureEditor(t, text)
	command(t, e, "add-simulation", "", nil)
	replacement := e.draft.Simulations[1].Name
	before := e.visualYAML()
	if err := e.mutateStructure("delete", "simulations.0", nil); err == nil || e.visualYAML() != before {
		t.Fatal("missing replacements changed draft")
	}
	choices := map[string]string{}
	for _, p := range e.references("model") {
		choices[p] = replacement
	}
	command(t, e, "delete", "simulations.0", choices)
	if len(e.draft.Simulations) != 1 || len(e.references(replacement)) != 3 {
		t.Fatal("cross-gateway references lost")
	}
	if len(e.draft.Gateways[0].Downstreams[1].Mappings) != 1 {
		t.Fatal("injector mapping lost")
	}
}
func TestModelMigrationRejectsInjectorConflictWithoutPartialChanges(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	command(t, e, "add-simulation", "", nil)
	other := e.draft.Simulations[1].Name
	command(t, e, "add-downstream", "gateways.0", nil)
	d := &e.draft.Gateways[0].Downstreams[3]
	d.Type = "injector"
	d.SlaveIDs = "3"
	d.SimulationRef = other
	d.Mappings = append(d.Mappings, e.draft.Gateways[0].Downstreams[1].Mappings[0])
	e.rebuild()
	before := e.visualYAML()
	ids := copyIDs(e.ids)
	choices := map[string]string{}
	for _, p := range e.references("model") {
		choices[p] = other
	}
	if err := e.mutateStructure("delete", "simulations.0", choices); err == nil {
		t.Fatal("accepted target overlap")
	}
	if e.visualYAML() != before || e.ids["simulations.0"] != ids["simulations.0"] || len(e.references("model")) != 2 {
		t.Fatal("failed migration mutated draft")
	}
}
func TestCascadeKeepsGatewaysAndUnrelatedModels(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	command(t, e, "add-simulation", "", nil)
	command(t, e, "cascade", "simulations.0", nil)
	if len(e.draft.Gateways) != 1 || len(e.draft.Gateways[0].Downstreams) != 1 || e.draft.Gateways[0].Downstreams[0].Name != "device" || len(e.draft.Simulations) != 1 {
		t.Fatal("cascade removed unrelated entities")
	}
	command(t, e, "delete", "gateways.0", nil)
	if len(e.draft.Simulations) != 1 || len(e.draft.Gateways) != 0 {
		t.Fatal("gateway deletion removed model")
	}
	if err := e.doSave(); err != nil {
		t.Fatalf("core-supported empty config rejected: %v", err)
	}
}
func TestStructurePreservesAdvancedFieldsAndComments(t *testing.T) {
	text := strings.Replace(liveConfig("127.0.0.1:15020"), "      - name: device", `      # serial settings must stay with this device
      - name: serial
        type: rtu
        slave_ids: "8"
        serial:
          device: /dev/test
          baud_rate: 19200
          data_bits: 7
          parity: E
          stop_bits: 2
          rs485: true
          timeout: 2s
      - name: device`, 1)
	e := structureEditor(t, text)
	command(t, e, "delete", "gateways.0.downstreams.0", nil)
	command(t, e, "add-upstream", "gateways.0", nil)
	yaml := e.visualYAML()
	cfg := parsed(t, yaml)
	serial := cfg.Gateways[0].Downstreams[1].Serial
	if serial.DataBits != 7 || serial.StopBits != 2 || serial.Parity != "E" || !serial.RS485 || !strings.Contains(yaml, "serial settings must stay") || !strings.Contains(yaml, "retain notes") {
		t.Fatalf("advanced fields lost: %+v\n%s", serial, yaml)
	}
	command(t, e, "delete", "gateways.0.upstreams.1", nil)
	if len(e.draft.Gateways[0].Upstreams) != 1 {
		t.Fatal("upstream deletion failed")
	}
}
func TestEmptyGroupsAndUniqueNames(t *testing.T) {
	e := structureEditor(t, "version: 1\nsimulations: []\ngateways: []\n")
	for i := 0; i < 2; i++ {
		command(t, e, "add-simulation", "", nil)
		command(t, e, "add-gateway", "", nil)
	}
	if e.draft.Simulations[0].Name == e.draft.Simulations[1].Name || e.draft.Gateways[0].Name == e.draft.Gateways[1].Name {
		t.Fatal("duplicate generated names")
	}
	if e.draft.Simulations[0].Persistence.Type != "memory" || len(editorProblems(e.draft)) == 0 {
		t.Fatal("incomplete draft treated as ready")
	}
	command(t, e, "add-upstream", "gateways.0", nil)
	command(t, e, "add-downstream", "gateways.0", nil)
}
func TestDefaultRouteMustBeResolvedBeforeAddingDownstream(t *testing.T) {
	e := structureEditor(t, `version: 1
simulations: []
gateways:
 - name: legacy
   upstreams: [{type: tcp, tcp: {address: "127.0.0.1:15020"}}]
   downstreams: [{name: device, type: tcp, tcp: {address: "127.0.0.1:15021"}}]
`)
	before := e.visualYAML()
	if err := e.mutateStructure("add-downstream", "gateways.0", nil); err == nil || e.visualYAML() != before {
		t.Fatal("default route silently changed")
	}
	e.draft.Gateways[0].Downstreams[0].SlaveIDs = "1"
	command(t, e, "add-downstream", "gateways.0", nil)
	if len(editorProblems(e.draft)) == 0 {
		t.Fatal("new unreachable downstream not flagged")
	}
}
func TestRenameDuplicateAndPendingInput(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	command(t, e, "add-simulation", "", nil)
	before := e.visualYAML()
	if err := e.mutateStructure("rename", "simulations.0", map[string]string{"name": e.draft.Simulations[1].Name}); err == nil || e.visualYAML() != before {
		t.Fatal("duplicate rename accepted")
	}
	e.eds["simulations.0.name"].SetText("pending")
	if !e.unsaved() {
		t.Fatal("pending rename not tracked")
	}
	if err := e.doSave(); err != nil {
		t.Fatal(err)
	}
	if len(e.references("pending")) != 2 || e.pendingRename() {
		t.Fatal("save did not complete rename atomically")
	}
}
func TestDesktopCompletenessLeavesLegacyRulesIntact(t *testing.T) {
	cfg := &config.Config{Version: 1}
	if len(editorProblems(cfg)) == 0 {
		t.Fatal("missing completeness problem")
	}
	if len(cfg.Problems()) != 0 {
		t.Fatal("CLI validation changed")
	}
	cfg.Version = 0
	if len(editorProblems(cfg)) != len(cfg.Problems()) {
		t.Fatal("legacy rules changed")
	}
}

func TestUndoStructurePreservesLaterFieldEdits(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	command(t, e, "delete", "gateways.0.downstreams.0", nil)
	e.draft.Gateways[0].Downstreams[1].Tcp.Address = "127.0.0.1:16000"
	if err := e.undoStructure(); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Gateways[0].Downstreams) != 3 || e.draft.Gateways[0].Downstreams[2].Tcp.Address != "127.0.0.1:16000" {
		t.Fatal("undo lost later edits")
	}
	if len(e.changes()) != 1 {
		t.Fatalf("undo diff: %+v", e.changes())
	}
}
func TestUndoModelCreationCannotLeaveDanglingReferences(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	command(t, e, "new-model", "gateways.0.downstreams.0", nil)
	e.draft.Gateways[0].Downstreams[1].SimulationRef = e.draft.Simulations[1].Name
	before := e.visualYAML()
	if err := e.undoStructure(); err == nil || e.visualYAML() != before || e.undoState == nil {
		t.Fatal("unsafe undo changed draft or lost retry")
	}
}
func TestRemovingIdenticalMappingStillCountsAsChange(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	d := &e.draft.Gateways[0].Downstreams[1]
	d.Mappings = append(d.Mappings, d.Mappings[0])
	e.rebuild()
	e.commitBaseline()
	d.Mappings = d.Mappings[:1]
	e.rebuild()
	if !e.unsaved() {
		t.Fatal("mapping removal disappeared from diff")
	}
}

func TestRawReorderRetainsNamedEntityIdentity(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	command(t, e, "add-simulation", "", nil)
	id := e.node("simulations.0").id
	cfg := parsed(t, e.visualYAML())
	cfg.Simulations[0], cfg.Simulations[1] = cfg.Simulations[1], cfg.Simulations[0]
	e.rawEd.SetText(toYAML(cfg))
	e.reparse()
	if e.node("simulations.1").id != id {
		t.Fatal("YAML reorder lost named identity")
	}
}
func TestEditorRejectsWildcardListenConflicts(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	command(t, e, "add-upstream", "gateways.0", nil)
	e.draft.Gateways[0].Upstreams[1].Tcp.Address = "0.0.0.0:15020"
	found := false
	for _, p := range editorProblems(e.draft) {
		found = found || strings.Contains(p.Message, "通配地址")
	}
	if !found {
		t.Fatal("wildcard bind conflict not detected")
	}
}

func TestStructureButtonsDriveEditorCommands(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	frame := func() {
		var ops op.Ops
		e.Layout(layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))})
	}
	frame()
	e.structure.get("add-simulation").Click()
	frame()
	if e.creation == nil || len(e.draft.Simulations) != 1 {
		t.Fatal("creation must open without mutating draft")
	}
	e.creation.name.SetText("new-model")
	if err := e.confirmCreation(); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Simulations) != 2 {
		t.Fatal("confirm model creation failed")
	}
	e.sel = "gateways.0"
	frame()
	e.structure.get("add-upstream|gateways.0").Click()
	frame()
	if len(e.draft.Gateways[0].Upstreams) != 2 {
		t.Fatal("add upstream button failed")
	}
	// Enter a value and click a structural action in the same update: preserve it.
	e.eds["gateways.0.name"].SetText("edited")
	e.structure.get("add-downstream|gateways.0").Click()
	frame()
	if e.draft.Gateways[0].Name != "edited" || len(e.draft.Gateways[0].Downstreams) != 4 {
		t.Fatal("structural click lost field input")
	}
	e.sel = "gateways.0.downstreams.3"
	frame()
	e.structure.get("request-delete|" + e.sel).Click()
	frame()
	if len(e.draft.Gateways[0].Downstreams) != 4 || e.deletePath == "" {
		t.Fatal("delete skipped preview")
	}
	e.structure.get("delete|" + e.sel).Click()
	frame()
	if len(e.draft.Gateways[0].Downstreams) != 3 {
		t.Fatal("delete confirmation failed")
	}
	e.undoBtn.Click()
	frame()
	if len(e.draft.Gateways[0].Downstreams) != 4 {
		t.Fatal("undo button failed")
	}
	e.revert.Click()
	frame()
	if len(e.changes()) != 0 || len(e.draft.Simulations) != 1 {
		t.Fatal("revert lost baseline identities")
	}
}

func TestPendingRenameSurvivesRebuildAndBlocksModeSwitch(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	e.eds["simulations.0.name"].SetText("pending")
	e.rebuild()
	if e.eds["simulations.0.name"].Text() != "pending" {
		t.Fatal("rebuild discarded pending name")
	}
	e.modes[1].Click()
	var ops op.Ops
	e.Layout(layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))})
	if e.raw || !e.pendingRename() {
		t.Fatal("mode switch discarded pending rename")
	}
	e.sel = "simulations.0"
	e.renameBtn.Click()
	ops.Reset()
	e.Layout(layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))})
	if e.pendingRename() || len(e.references("pending")) != 2 {
		t.Fatal("rename button did not update references")
	}
}

func TestInvalidDuplicateNamesNeverShareEditorIdentity(t *testing.T) {
	e := structureEditor(t, liveConfig("127.0.0.1:15020"))
	command(t, e, "add-simulation", "", nil)
	cfg := parsed(t, e.visualYAML())
	cfg.Simulations[1].Name = cfg.Simulations[0].Name
	e.rawEd.SetText(toYAML(cfg))
	e.reparse()
	if e.node("simulations.0").id == e.node("simulations.1").id {
		t.Fatal("duplicate names share editor identity")
	}
	if e.doSave() == nil {
		t.Fatal("duplicate model names saved")
	}
}
