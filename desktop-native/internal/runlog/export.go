package runlog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ffutop/modbus-gateway/internal/config"
)

// Export writes the visible records. Existing files require explicit overwrite.
// Creating a new target links a completed private temporary file exclusively.
func Export(path, content string, overwrite bool) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("请输入日志导出路径")
	}
	if overwrite {
		return config.WriteFile(path, []byte(content))
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".modmux-log-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Link(f.Name(), path)
}
