package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/filepicker"
	"github.com/ffutop/modbus-gateway/internal/config"
)

func blankEditor(t *testing.T, save func(string) error) *configEditor {
	t.Helper()
	cfg, _ := config.ParseDraft([]byte("version: 1\n"))
	cfg.Path = filepath.Join(t.TempDir(), "config.yaml")
	return newConfigEditor(NewTheme(), Info{Config: cfg, NewFile: true, Save: save})
}

func TestNewFileStartsAsVisualV1DraftAndSavesWhatWasBuilt(t *testing.T) {
	var written string
	e := blankEditor(t, func(text string) error { written = text; return nil })
	if e.raw || e.visualLocked || e.draft.Version != 1 || !e.newFile {
		t.Fatalf("blank file not editable visually: raw=%v locked=%v v%d", e.raw, e.visualLocked, e.draft.Version)
	}
	if e.unsaved() {
		t.Fatal("an untouched blank file has nothing to save")
	}
	command(t, e, "add-simulation", "", nil)
	if err := e.doSave(); err != nil {
		t.Fatal(err)
	}
	cfg := parsed(t, written)
	if cfg.Version != 1 || len(cfg.Simulations) != 1 || e.newFile {
		t.Fatalf("saved:\n%s", written)
	}
}

func TestFileDialogListsDirectoriesAndYAMLOnly(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.yaml", "a.YML", "notes.txt", ".hidden.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sites"), 0700); err != nil {
		t.Fatal(err)
	}
	e := blankEditor(t, nil)
	e.configPath = filepath.Join(dir, "config.yaml")
	e.openFileDialog(false)
	var names []string
	for _, entry := range e.files.entries {
		names = append(names, entry.Name())
	}
	if strings.Join(names, ",") != "sites,a.YML,b.yaml" {
		t.Fatalf("entries: %v", names)
	}
	e.files.name.SetText("sites")
	if err := e.confirmFile(); err != nil || e.files == nil || e.files.dir != filepath.Join(dir, "sites") {
		t.Fatalf("naming a directory should enter it: %v", err)
	}
}

func TestOpenRefusesToDropUnsavedEdits(t *testing.T) {
	var opened string
	e := blankEditor(t, nil)
	e.openFile = func(path string) error { opened = path; return nil }
	command(t, e, "add-simulation", "", nil)
	e.openFileDialog(false)
	e.files.name.SetText("other.yaml")
	if err := e.confirmFile(); err == nil || opened != "" {
		t.Fatal("open discarded unsaved edits")
	}
	e.saveFile = func(string) error { return nil }
	if err := e.doSave(); err != nil {
		t.Fatal(err)
	}
	dir := e.files.dir
	if err := e.confirmFile(); err != nil || opened != filepath.Join(dir, "other.yaml") || e.files != nil {
		t.Fatalf("open after save: %q %v", opened, err)
	}
}

func TestSaveAsAddsExtensionAndConfirmsOverwrite(t *testing.T) {
	dir := t.TempDir()
	var path, text string
	e := blankEditor(t, nil)
	e.configPath = filepath.Join(dir, "config.yaml")
	e.saveAsFile = func(p, t string) error { path, text = p, t; return nil }
	command(t, e, "add-simulation", "", nil)
	e.openFileDialog(true)
	if e.files.name.Text() != "config.yaml" {
		t.Fatal("save as should suggest the current name")
	}
	e.files.name.SetText("site")
	if err := e.confirmFile(); err != nil || path != filepath.Join(dir, "site.yaml") || len(parsed(t, text).Simulations) != 1 {
		t.Fatalf("save as: %q %v", path, err)
	}

	path = ""
	existing := filepath.Join(dir, "existing.yaml")
	if err := os.WriteFile(existing, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	e.openFileDialog(true)
	e.files.name.SetText("existing.yaml")
	if err := e.confirmFile(); err != nil || path != "" || e.files.overwrite != existing {
		t.Fatalf("overwrote without confirmation: %q %v", path, err)
	}
	if err := e.confirmFile(); err != nil || path != existing {
		t.Fatalf("confirmed overwrite: %q %v", path, err)
	}
}

func TestFirstEntryOfMissingListIsKept(t *testing.T) {
	e := structureEditor(t, "version: 1\n")
	command(t, e, "add-simulation", "", nil)
	command(t, e, "add-gateway", "", nil)
	cfg := parsed(t, e.visualYAML())
	if len(cfg.Simulations) != 1 || len(cfg.Gateways) != 1 {
		t.Fatalf("first entries lost:\n%s", e.visualYAML())
	}
}

func TestNewFileHasNothingToApply(t *testing.T) {
	e := blankEditor(t, nil)
	if len(e.pendingChanges()) != 0 {
		t.Fatalf("pending: %+v", e.pendingChanges())
	}
}

// pickWith runs chooseFile with a fake system chooser and applies its result.
func pickWith(t *testing.T, e *configEditor, saveAs bool, path string, err error) {
	t.Helper()
	gtx := layout.Context{Ops: new(op.Ops)}
	e.pickFile = func(bool, string, string) (string, error) { return path, err }
	e.chooseFile(gtx, saveAs)
	p := e.picking
	if p == nil {
		return // refused or fell back before asking
	}
	r := <-p.done
	e.picking = nil
	e.finishPick(gtx, p, r)
}

func TestSystemChooserOpensSavesAndFallsBack(t *testing.T) {
	dir := t.TempDir()
	var opened, savedAs, text string
	e := blankEditor(t, nil)
	e.configPath = filepath.Join(dir, "config.yaml")
	e.openFile = func(p string) error { opened = p; return nil }
	e.saveAsFile = func(p, t string) error { savedAs, text = p, t; return nil }

	pickWith(t, e, false, "", nil)
	if opened != "" || e.toastErr || e.files != nil {
		t.Fatal("cancel must change nothing")
	}
	pickWith(t, e, false, filepath.Join(dir, "a.yaml"), nil)
	if opened != filepath.Join(dir, "a.yaml") {
		t.Fatalf("opened %q", opened)
	}
	pickWith(t, e, false, "", filepicker.ErrUnavailable)
	if e.files == nil || e.files.saveAs {
		t.Fatal("no system chooser should fall back to the in-app dialog")
	}
	e.files = nil

	command(t, e, "add-simulation", "", nil)
	opened = ""
	pickWith(t, e, false, filepath.Join(dir, "b.yaml"), nil)
	if opened != "" || !e.toastErr {
		t.Fatal("open must refuse unsaved edits before asking")
	}
	pickWith(t, e, true, filepath.Join(dir, "site"), nil)
	if savedAs != filepath.Join(dir, "site.yaml") || len(parsed(t, text).Simulations) != 1 {
		t.Fatalf("save as %q", savedAs)
	}
	savedAs = ""
	if err := os.WriteFile(filepath.Join(dir, "taken.yaml"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	pickWith(t, e, true, filepath.Join(dir, "taken"), nil)
	if savedAs != "" || !e.toastErr {
		t.Fatal("an appended extension must not replace an unconfirmed file")
	}
	pickWith(t, e, true, "", errors.New("boom"))
	if !e.toastErr || !strings.Contains(e.toast, "boom") {
		t.Fatalf("chooser error not shown: %q", e.toast)
	}
}
