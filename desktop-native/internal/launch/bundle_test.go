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

func TestArchiveLaunchUsesPlatformUserDirectories(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "modmux-desktop-linux-amd64", "modmux-desktop")
	if err := os.MkdirAll(filepath.Dir(exe), 0755); err != nil {
		t.Fatal(err)
	}
	sample := []byte("version: 1\n")
	if err := os.WriteFile(filepath.Join(filepath.Dir(exe), Sample), sample, 0644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	for _, tc := range []struct {
		name, goos  string
		vars        map[string]string
		config, log string
	}{
		{"linux defaults", "linux", nil,
			filepath.Join(home, ".config", "modmux", "config.yaml"),
			filepath.Join(home, ".local", "state", "modmux", "desktop.log")},
		{"linux XDG", "linux", map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "xdg-config"), "XDG_STATE_HOME": filepath.Join(dir, "xdg-state")},
			filepath.Join(dir, "xdg-config", "modmux", "config.yaml"),
			filepath.Join(dir, "xdg-state", "modmux", "desktop.log")},
		{"linux ignores relative XDG", "linux", map[string]string{"XDG_CONFIG_HOME": "relative"},
			filepath.Join(home, ".config", "modmux", "config.yaml"),
			filepath.Join(home, ".local", "state", "modmux", "desktop.log")},
		{"windows AppData", "windows", map[string]string{"APPDATA": filepath.Join(dir, "Roaming"), "LOCALAPPDATA": filepath.Join(dir, "Local")},
			filepath.Join(dir, "Roaming", "ModMux", "config.yaml"),
			filepath.Join(dir, "Local", "ModMux", "Logs", "desktop.log")},
		{"windows defaults", "windows", nil,
			filepath.Join(home, "AppData", "Roaming", "ModMux", "config.yaml"),
			filepath.Join(home, "AppData", "Local", "ModMux", "Logs", "desktop.log")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := prepare(exe, home, "", tc.goos, env(tc.vars))
			if err != nil {
				t.Fatal(err)
			}
			if p.Config != tc.config || p.Log != tc.log || p.WorkDir != filepath.Dir(tc.config) {
				t.Fatalf("paths: %+v", p)
			}
			if b, err := os.ReadFile(p.Config); err != nil || string(b) != string(sample) {
				t.Fatalf("bad seed: %q %v", b, err)
			}
			if _, err := os.Stat(filepath.Dir(p.Log)); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := prepare(exe, "", "", "linux", env(nil)); err == nil {
		t.Fatal("a missing home directory needs an XDG override")
	}
	if p, err := prepare(exe, "", "", "windows", env(map[string]string{"APPDATA": filepath.Join(dir, "R"), "LOCALAPPDATA": filepath.Join(dir, "L")})); err != nil || p.Config == "" {
		t.Fatalf("AppData needs no home directory: %+v %v", p, err)
	}
}

func TestExecutableWithoutSampleIsACommandLineLaunch(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "modmux-desktop")
	// A directory of that name is not a shipped sample.
	if err := os.Mkdir(filepath.Join(dir, Sample), 0755); err != nil {
		t.Fatal(err)
	}
	for _, goos := range []string{"linux", "windows", "darwin"} {
		p, err := prepare(exe, dir, "", goos, os.Getenv)
		if err != nil || p != (Paths{}) {
			t.Fatalf("%s: %+v %v", goos, p, err)
		}
	}
}
