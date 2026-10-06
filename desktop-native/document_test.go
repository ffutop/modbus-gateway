package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/sidecar"
)

const sample = "version: 1\nsimulations:\n  - name: m\n    persistence: {type: memory}\n"

func TestDocumentSwitchesOnSaveAsAndOpen(t *testing.T) {
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	dir := t.TempDir()
	first := filepath.Join(dir, "config.yaml")
	sup := sidecar.NewSupervisor("unused", first, nil)
	doc := newDocument(sup, nil)
	if err := doc.switchTo(first, "", ""); err != nil {
		t.Fatal(err)
	}
	doc.recovery().Queue("draft")

	sub := filepath.Join(dir, "site")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	gen := doc.generation()
	if err := doc.SaveAs(filepath.Join(sub, "copy.txt"), sample); err == nil {
		t.Fatal("non-YAML name accepted")
	}
	copyPath := filepath.Join(sub, "copy.yaml")
	if err := doc.SaveAs(copyPath, sample); err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(copyPath)
	if doc.path() != real || doc.generation() == gen || doc.takeNotice() == "" {
		t.Fatalf("not switched: %s", doc.path())
	}
	if cwd, _ := os.Getwd(); cwd != filepath.Dir(real) {
		t.Fatalf("relative paths must resolve beside the file, cwd %s", cwd)
	}
	if b, _ := os.ReadFile(copyPath); string(b) != sample {
		t.Fatal("draft not written")
	}

	if err := doc.Open(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("opened a missing file")
	}
	if err := doc.Open(sub); err == nil {
		t.Fatal("opened a directory")
	}
	broken := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(broken, []byte("version: ["), 0600); err != nil {
		t.Fatal(err)
	}
	if err := doc.Open(broken); err != nil {
		t.Fatal(err) // invalid files open for repair, without starting the gateway
	}
	if real, _ := filepath.EvalSymlinks(broken); doc.path() != real {
		t.Fatal("open did not switch")
	}
}
