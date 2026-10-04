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
		return fmt.Errorf("配置文件已被其他程序修改；本次未保存，请重新打开应用后合并修改")
	}
	if err := config.WriteFile(f.Path, []byte(text)); err != nil {
		return err
	}
	f.Content, f.revision = text, config.Revision([]byte(text))
	return nil
}
