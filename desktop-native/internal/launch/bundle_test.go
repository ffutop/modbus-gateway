package launch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigIsBesideTheApp(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ exe, config string }{
		{filepath.Join(dir, "modmux-desktop"), filepath.Join(dir, "config.yaml")},
		{filepath.Join(dir, "App with spaces.app", "Contents", "MacOS", "modmux-desktop"), filepath.Join(dir, "config.yaml")},
	} {
		p, err := prepare(tc.exe, dir, "", false, "darwin", os.Getenv)
		if err != nil || p.Config != tc.config {
			t.Fatalf("%s: %+v %v", tc.exe, p, err)
		}
		if _, err := os.Stat(p.Config); !os.IsNotExist(err) {
			t.Fatal("no configuration may be created at launch")
		}
	}
}

func TestExplicitConfigIsAbsolute(t *testing.T) {
	p, err := prepare("/tmp/modmux-desktop", "", "relative.yaml", false, "linux", os.Getenv)
	want, _ := filepath.Abs("relative.yaml")
	if err != nil || p.Config != want || p.Log != "" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestPackagedAppLogsToUserDirectories(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	exe := filepath.Join(dir, "modmux-desktop")
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	for _, tc := range []struct {
		name, goos string
		vars       map[string]string
		log        string
	}{
		{"macOS", "darwin", nil, filepath.Join(home, "Library", "Logs", "ModMux", "desktop.log")},
		{"linux defaults", "linux", nil, filepath.Join(home, ".local", "state", "modmux", "desktop.log")},
		{"linux XDG", "linux", map[string]string{"XDG_STATE_HOME": filepath.Join(dir, "xdg-state")}, filepath.Join(dir, "xdg-state", "modmux", "desktop.log")},
		{"linux ignores relative XDG", "linux", map[string]string{"XDG_STATE_HOME": "relative"}, filepath.Join(home, ".local", "state", "modmux", "desktop.log")},
		{"windows AppData", "windows", map[string]string{"LOCALAPPDATA": filepath.Join(dir, "Local")}, filepath.Join(dir, "Local", "ModMux", "Logs", "desktop.log")},
		{"windows defaults", "windows", nil, filepath.Join(home, "AppData", "Local", "ModMux", "Logs", "desktop.log")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := prepare(exe, home, "", true, tc.goos, env(tc.vars))
			if err != nil || p.Log != tc.log || p.Config != filepath.Join(dir, ConfigName) {
				t.Fatalf("paths: %+v %v", p, err)
			}
			if _, err := os.Stat(filepath.Dir(p.Log)); err != nil {
				t.Fatal(err)
			}
		})
	}
	bundle := filepath.Join(dir, "ModMux.app", "Contents", "MacOS", "modmux-desktop")
	if p, err := prepare(bundle, home, "", false, "darwin", env(nil)); err != nil || p.Log == "" {
		t.Fatalf("a bundle is packaged: %+v %v", p, err)
	}
	if _, err := prepare(exe, "", "", true, "linux", env(nil)); err == nil {
		t.Fatal("a missing home directory needs an XDG override")
	}
	if p, err := prepare(exe, "", "", true, "windows", env(map[string]string{"LOCALAPPDATA": filepath.Join(dir, "L")})); err != nil || p.Log == "" {
		t.Fatalf("AppData needs no home directory: %+v %v", p, err)
	}
}
