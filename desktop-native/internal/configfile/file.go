// Package configfile owns the desktop draft's optimistic save boundary.
package configfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ffutop/modbus-gateway/internal/config"
)

// File is the configuration file the editor shows. It may not exist yet: its
// first save then creates it, but never over a file created meanwhile.
type File struct {
	mu       sync.Mutex
	Path     string
	Content  string
	revision string // empty while the file does not exist
}

func Open(path string) (*File, error) {
	abs, err := resolve(path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return &File{Path: abs}, nil
	}
	if err != nil {
		return nil, err
	}
	return &File{Path: abs, Content: string(b), revision: config.Revision(b)}, nil
}

// SaveAs writes text to path, replacing any file there, and returns it as the
// file to edit from now on.
func SaveAs(path, text string) (*File, error) {
	if err := check(text); err != nil {
		return nil, err
	}
	abs, err := resolve(path)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return nil, fmt.Errorf("%s 是目录", abs)
	}
	if err := config.WriteFile(abs, []byte(text)); err != nil {
		return nil, err
	}
	return &File{Path: abs, Content: text, revision: config.Revision([]byte(text))}, nil
}

// Exists reports whether the file existed when last read or written here.
func (f *File) Exists() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.revision != "" }

// resolve keeps a user-selected symlink intact when atomically replacing its
// target. A missing file resolves within its resolved directory.
func resolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if errors.Is(err, os.ErrNotExist) {
		if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
			return filepath.Join(dir, filepath.Base(abs)), nil
		}
		return abs, nil
	}
	return real, err
}

func check(text string) error {
	cfg, err := config.ParseDraft([]byte(text))
	if err != nil {
		return err
	}
	if problems := cfg.Problems(); len(problems) > 0 {
		return fmt.Errorf("%s", problems[0].Message)
	}
	return nil
}

func (f *File) Save(text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := check(text); err != nil {
		return err
	}
	onDisk, err := os.ReadFile(f.Path)
	if f.revision == "" {
		if err == nil {
			return &Conflict{Baseline: f.Content, Disk: string(onDisk), Draft: text}
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := create(f.Path, []byte(text)); err != nil {
			return err
		}
		f.Content, f.revision = text, config.Revision([]byte(text))
		return nil
	}
	if err != nil {
		return err
	}
	if config.Revision(onDisk) != f.revision {
		return &Conflict{Baseline: f.Content, Disk: string(onDisk), Draft: text}
	}
	if err := config.WriteFile(f.Path, []byte(text)); err != nil {
		return err
	}
	f.Content, f.revision = text, config.Revision([]byte(text))
	return nil
}

// create writes a new file, failing if one appeared since the check.
func create(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		onDisk, _ := os.ReadFile(path)
		return &Conflict{Disk: string(onDisk), Draft: string(content)}
	}
	if err != nil {
		return err
	}
	if _, err = file.Write(content); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	return file.Close()
}

// Conflict preserves all three texts without changing the optimistic baseline.
type Conflict struct{ Baseline, Disk, Draft string }

func (c *Conflict) Error() string {
	return "配置文件已被其他程序修改；请查看三份文本并重新基准化后合并"
}

// Rebase accepts only the exact external version the user reviewed. A subsequent
// external edit will still be rejected by Save's regular revision check.
func (f *File) Rebase(reviewed string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	current, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) && f.revision == "" && reviewed == "" {
		return nil
	}
	if err != nil {
		return err
	}
	if string(current) != reviewed {
		return &Conflict{Baseline: f.Content, Disk: string(current)}
	}
	f.Content, f.revision = reviewed, config.Revision(current)
	return nil
}
