package configfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const valid = `version: 1
# keep deployment notes
gateways:
  - name: demo
    upstreams:
      - type: tcp
        tcp: {address: "127.0.0.1:15020"}
    downstreams:
      - name: meter
        type: tcp
        slave_ids: "1"
        tcp: {address: "127.0.0.1:15021"}
`

func fixture(t *testing.T) *File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSavePreservesRawTextPermissionsAndSupportsRepeatedSaves(t *testing.T) {
	f := fixture(t)
	for _, name := range []string{"first", "second"} {
		text := strings.Replace(valid, "name: demo", "name: "+name, 1)
		if err := f.Save(text); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(f.Path)
		if err != nil || string(b) != text {
			t.Fatalf("raw text not preserved: %q, %v", b, err)
		}
	}
	stat, err := os.Stat(f.Path)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatalf("permissions changed: %v, %v", stat, err)
	}
}

func TestSaveRejectsInvalidOrExternallyChangedFile(t *testing.T) {
	for _, text := range []string{"version: [", strings.Replace(valid, `slave_ids: "1"`, `slave_ids: "999"`, 1)} {
		f := fixture(t)
		if err := f.Save(text); err == nil {
			t.Fatal("invalid draft saved")
		}
		b, _ := os.ReadFile(f.Path)
		if string(b) != valid || f.Content != valid {
			t.Fatal("failed save changed baseline")
		}
	}
	f := fixture(t)
	external := valid + "# edited elsewhere\n"
	if err := os.WriteFile(f.Path, []byte(external), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(valid + "# UI edit\n"); err == nil {
		t.Fatal("external edit overwritten")
	}
	b, _ := os.ReadFile(f.Path)
	if string(b) != external || f.Content != valid {
		t.Fatal("conflict changed disk or baseline")
	}
}

func TestSaveKeepsSymlinkAndLegacySchema(t *testing.T) {
	f := fixture(t)
	link := filepath.Join(filepath.Dir(f.Path), "selected.yaml")
	if err := os.Symlink(f.Path, link); err != nil {
		t.Skip(err)
	}
	linked, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.TrimPrefix(valid, "version: 1\n")
	if err := linked.Save(legacy); err != nil {
		t.Fatal(err)
	}
	if stat, err := os.Lstat(link); err != nil || stat.Mode()&os.ModeSymlink == 0 {
		t.Fatal("selected symlink replaced")
	}
	b, _ := os.ReadFile(link)
	if string(b) != legacy {
		t.Fatal("legacy text upgraded")
	}
}

func TestRebaseRequiresReviewedDiskAndChecksNextExternalEdit(t *testing.T) {
	f := fixture(t)
	draft := valid + "# draft\n"
	external := valid + "# external\n"
	if err := os.WriteFile(f.Path, []byte(external), 0600); err != nil {
		t.Fatal(err)
	}
	err := f.Save(draft)
	var conflict *Conflict
	if !errors.As(err, &conflict) || conflict.Baseline != valid || conflict.Disk != external || conflict.Draft != draft {
		t.Fatalf("missing three-way conflict: %v", err)
	}
	if f.Content != valid {
		t.Fatal("viewing conflict changed baseline")
	}
	if err := f.Rebase(external); err != nil {
		t.Fatal(err)
	}
	newer := external + "# changed again\n"
	if err := os.WriteFile(f.Path, []byte(newer), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(draft); err == nil {
		t.Fatal("second external edit overwritten")
	}
	if err := f.Rebase(external); err == nil {
		t.Fatal("unreviewed disk accepted")
	}
	if err := f.Rebase(newer); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(draft); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f.Path); string(b) != draft {
		t.Fatal("merged draft not saved")
	}
}

func TestMissingFileIsCreatedOnFirstSaveOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	f, err := Open(path)
	if err != nil || f.Exists() || f.Content != "" {
		t.Fatalf("missing file: %+v %v", f, err)
	}
	if err := f.Save(valid); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != valid || !f.Exists() {
		t.Fatalf("not created: %q", b)
	}

	other := filepath.Join(t.TempDir(), "config.yaml")
	g, _ := Open(other)
	if err := os.WriteFile(other, []byte("# created elsewhere\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var conflict *Conflict
	if err := g.Save(valid); !errors.As(err, &conflict) || conflict.Disk != "# created elsewhere\n" {
		t.Fatalf("file created meanwhile overwritten: %v", err)
	}
}

func TestSaveAsReplacesTargetAndBecomesTheFile(t *testing.T) {
	f := fixture(t)
	target := filepath.Join(t.TempDir(), "copy.yaml")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(valid, "name: demo", "name: copy", 1)
	g, err := SaveAs(target, text)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != text || g.Content != text || !g.Exists() {
		t.Fatalf("save as: %q", b)
	}
	if b, _ := os.ReadFile(f.Path); string(b) != valid {
		t.Fatal("source file changed")
	}
	if err := g.Save(text + "# next\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveAs(target, "version: ["); err == nil {
		t.Fatal("invalid draft saved")
	}
	if _, err := SaveAs(t.TempDir(), valid); err == nil {
		t.Fatal("directory replaced")
	}
}
