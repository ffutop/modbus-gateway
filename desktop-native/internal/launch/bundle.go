// Package launch resolves where the desktop app keeps its configuration and
// log when it starts.
package launch

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Paths struct {
	Config string // absolute; the file may not exist yet
	Log    string // empty: log to stderr
}

// ConfigName is the configuration the app opens by default, in AppDir.
const ConfigName = "config.yaml"

// Prepare resolves the configuration to open first: the -config file if
// given, otherwise ConfigName in AppDir, which the editor creates on the first
// save. A packaged app — a macOS bundle, or a build marked packaged — has no
// terminal, so it logs to the platform's user log directory.
func Prepare(executable, home, explicitConfig string, packaged bool) (Paths, error) {
	return prepare(executable, home, explicitConfig, packaged, runtime.GOOS, os.Getenv)
}

func prepare(executable, home, explicitConfig string, packaged bool, goos string, getenv func(string) string) (Paths, error) {
	config := explicitConfig
	if config == "" {
		config = filepath.Join(AppDir(executable), ConfigName)
	}
	config, err := filepath.Abs(config)
	if err != nil {
		return Paths{}, err
	}
	p := Paths{Config: config}
	if !packaged && !inBundle(executable) {
		return p, nil
	}
	if p.Log, err = logPath(goos, home, getenv); err != nil {
		return p, err
	}
	return p, os.MkdirAll(filepath.Dir(p.Log), 0700)
}

// AppDir is the directory users see the app in: the one holding the
// executable, or the one holding a macOS bundle, whose signed contents must
// stay unchanged.
func AppDir(executable string) string {
	if inBundle(executable) {
		return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(executable))))
	}
	return filepath.Dir(executable)
}

func inBundle(executable string) bool {
	dir := filepath.Dir(executable)
	contents := filepath.Dir(dir)
	return filepath.Base(dir) == "MacOS" && filepath.Base(contents) == "Contents" && strings.HasSuffix(filepath.Dir(contents), ".app")
}

// logPath follows each platform's convention: Logs on macOS, local AppData
// on Windows, and the XDG state directory elsewhere.
func logPath(goos, home string, getenv func(string) string) (string, error) {
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
	var log string
	switch goos {
	case "darwin":
		log = filepath.Join(home, "Library", "Logs", "ModMux", "desktop.log")
	case "windows":
		log = filepath.Join(dir("LOCALAPPDATA", "AppData", "Local"), "ModMux", "Logs", "desktop.log")
	default:
		log = filepath.Join(dir("XDG_STATE_HOME", ".local", "state"), "modmux", "desktop.log")
	}
	if missingHome {
		return "", fmt.Errorf("cannot locate user home directory")
	}
	return log, nil
}
