package launch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestBundlePreparesPrivateConfigAndPreservesEdits(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "App with spaces.app", "Contents", "MacOS", "modmux-desktop")
	resources := filepath.Join(filepath.Dir(filepath.Dir(exe)), "Resources")
	if err := os.MkdirAll(resources, 0755); err != nil {
		t.Fatal(err)
	}
	sample := []byte("version: 1\n# first-run sample\n")
	if err := os.WriteFile(filepath.Join(resources, "config.default.yaml"), sample, 0644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "user home")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Prepare(exe, home, ""); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	p, err := Prepare(exe, home, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p.Config)
	if err != nil || string(b) != string(sample) {
		t.Fatalf("bad seed: %q %v", b, err)
	}
	stat, err := os.Stat(p.Config)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("configuration must be private")
	}
	edited := []byte("# user edited this\n")
	if err := os.WriteFile(p.Config, edited, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(exe, home, ""); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p.Config)
	if string(b) != string(edited) {
		t.Fatal("upgrade overwrote user configuration")
	}
	if p.WorkDir != filepath.Dir(p.Config) || p.Log != filepath.Join(home, "Library", "Logs", "ModMux", "desktop.log") {
		t.Fatal(p)
	}
}

func TestCLILaunchIsUnchanged(t *testing.T) {
	for _, tc := range []struct{ exe, config string }{{"/tmp/modmux-desktop", ""}, {"/tmp/ModMux.app/Contents/MacOS/modmux-desktop", "relative.yaml"}} {
		p, err := Prepare(tc.exe, "", tc.config)
		if err != nil || p.Config != tc.config || p.WorkDir != "" || p.Log != "" {
			t.Fatalf("CLI changed: %+v %v", p, err)
		}
	}
}
