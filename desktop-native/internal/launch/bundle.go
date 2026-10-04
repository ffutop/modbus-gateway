// Package launch prepares writable user data for a packaged app launched from
// the desktop: a macOS app bundle, or a Windows or Linux archive.
package launch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Paths struct {
	Config  string
	WorkDir string
	Log     string
}

// Sample is the first-run configuration a package ships: in Contents/Resources
// of a macOS bundle, beside the executable in a Windows or Linux archive.
const Sample = "config.default.yaml"

// Prepare leaves command-line launches unchanged. A packaged app gets a private
// editable copy of its sample configuration in the user's directories,
// separate from the installed (on macOS, signed) files.
func Prepare(executable, home, explicitConfig string) (Paths, error) {
	return prepare(executable, home, explicitConfig, runtime.GOOS, os.Getenv)
}

func prepare(executable, home, explicitConfig, goos string, getenv func(string) string) (Paths, error) {
	if explicitConfig != "" {
		return Paths{Config: explicitConfig}, nil
	}
	sample, ok := packagedSample(executable)
	if !ok {
		return Paths{}, nil
	}
	p, err := userPaths(goos, home, getenv)
	if err != nil {
		return p, err
	}
	if err := os.MkdirAll(p.WorkDir, 0700); err != nil {
		return p, err
	}
	if _, err := os.Lstat(p.Config); errors.Is(err, os.ErrNotExist) {
		content, err := os.ReadFile(sample)
		if err != nil {
			return p, fmt.Errorf("read bundled configuration: %w", err)
		}
		if err := seed(p.Config, content); err != nil {
			return p, err
		}
	} else if err != nil {
		return p, err
	}
	if err := os.MkdirAll(filepath.Dir(p.Log), 0700); err != nil {
		return p, err
	}
	return p, nil
}

// packagedSample reports where a packaged app keeps its sample configuration.
// A macOS bundle is recognized by its layout, so a missing sample is an error
// at read time; elsewhere the sample beside the executable marks the package.
func packagedSample(executable string) (string, bool) {
	dir := filepath.Dir(executable)
	contents := filepath.Dir(dir)
	if filepath.Base(dir) == "MacOS" && filepath.Base(contents) == "Contents" && strings.HasSuffix(filepath.Dir(contents), ".app") {
		return filepath.Join(contents, "Resources", Sample), true
	}
	sample := filepath.Join(dir, Sample)
	if info, err := os.Stat(sample); err == nil && info.Mode().IsRegular() {
		return sample, true
	}
	return "", false
}

// userPaths follows each platform's convention: Application Support and Logs
// on macOS, roaming AppData for the configuration and local AppData for logs
// on Windows, and the XDG base directories elsewhere. Relative persistence
// paths resolve beside the configuration.
func userPaths(goos, home string, getenv func(string) string) (Paths, error) {
	// dir returns the absolute directory an environment variable names, or the
	// fallback under the home directory.
	missingHome := goos == "darwin" && home == ""
	dir := func(env string, fallback ...string) string {
		if v := getenv(env); v != "" && filepath.IsAbs(v) {
			return v
		}
		if home == "" {
			missingHome = true
		}
		return filepath.Join(append([]string{home}, fallback...)...)
	}
	var config, log string
	switch goos {
	case "darwin":
		config = filepath.Join(home, "Library", "Application Support", "ModMux")
		log = filepath.Join(home, "Library", "Logs", "ModMux", "desktop.log")
	case "windows":
		config = filepath.Join(dir("APPDATA", "AppData", "Roaming"), "ModMux")
		log = filepath.Join(dir("LOCALAPPDATA", "AppData", "Local"), "ModMux", "Logs", "desktop.log")
	default:
		config = filepath.Join(dir("XDG_CONFIG_HOME", ".config"), "modmux")
		log = filepath.Join(dir("XDG_STATE_HOME", ".local", "state"), "modmux", "desktop.log")
	}
	if missingHome {
		return Paths{}, fmt.Errorf("cannot locate user home directory")
	}
	return Paths{Config: filepath.Join(config, "config.yaml"), WorkDir: config, Log: log}, nil
}

// Publish a complete first-run file without overwriting an existing file, even
// if two launches initialize the same data directory concurrently.
func seed(path string, content []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".config-initial-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(content); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Link(f.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}
