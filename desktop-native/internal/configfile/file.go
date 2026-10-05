// Package configfile owns the desktop draft's optimistic save boundary.
package configfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ffutop/modbus-gateway/internal/config"
)

type File struct {
	mu       sync.Mutex
	Path     string
	Content  string
	revision string
}

func Open(path string) (*File, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Keep a user-selected symlink intact when atomically replacing its target.
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return &File{Path: abs, Content: string(b), revision: config.Revision(b)}, nil
}

func (f *File) Save(text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cfg, err := config.ParseDraft([]byte(text))
	if err != nil {
		return err
	}
	if problems := cfg.Problems(); len(problems) > 0 {
		return fmt.Errorf("%s", problems[0].Message)
	}
	onDisk, err := os.ReadFile(f.Path)
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
	if err != nil {
		return err
	}
	if string(current) != reviewed {
		return &Conflict{Baseline: f.Content, Disk: string(current)}
	}
	f.Content, f.revision = reviewed, config.Revision(current)
	return nil
}
