// Package launch prepares writable user data for a Finder-launched app bundle.
package launch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	Config  string
	WorkDir string
	Log     string
}

// Prepare leaves command-line launches unchanged. An app bundle gets a private
// editable copy of its sample configuration, separate from signed resources.
func Prepare(executable, home, explicitConfig string) (Paths, error) {
	if explicitConfig != "" {
		return Paths{Config: explicitConfig}, nil
	}
	macOS := filepath.Dir(executable)
	contents := filepath.Dir(macOS)
	if filepath.Base(macOS) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(filepath.Dir(contents), ".app") {
		return Paths{}, nil
	}
	if home == "" {
		return Paths{}, fmt.Errorf("cannot locate user home directory")
	}
	p := Paths{
		Config:  filepath.Join(home, "Library", "Application Support", "ModMux", "config.yaml"),
		WorkDir: filepath.Join(home, "Library", "Application Support", "ModMux"),
		Log:     filepath.Join(home, "Library", "Logs", "ModMux", "desktop.log"),
	}
	if err := os.MkdirAll(p.WorkDir, 0700); err != nil {
		return p, err
	}
	if _, err := os.Lstat(p.Config); errors.Is(err, os.ErrNotExist) {
		content, err := os.ReadFile(filepath.Join(contents, "Resources", "config.default.yaml"))
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
