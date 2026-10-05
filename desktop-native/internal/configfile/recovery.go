package configfile

import (
	"encoding/json"
	"github.com/ffutop/modbus-gateway/internal/config"
	"os"
	"path/filepath"
	"sync"
)

// Recovery serializes asynchronous autosaves and invalidates queued old writes
// when a configuration is saved or a recovery record is discarded.
type Recovery struct {
	mu                   sync.Mutex
	path, base           string
	baseText, loadedBase string
	generation           uint64
	sequence             uint64
	err                  error
}
type recoveryRecord struct {
	Version  int    `json:"version"`
	Baseline string `json:"baseline,omitempty"`
	Base     string `json:"base_revision"`
	Content  string `json:"content"`
}

func NewRecovery(path, content string) *Recovery {
	return &Recovery{path: path + ".modmux-draft.json", base: config.Revision([]byte(content)), baseText: content}
}
func (r *Recovery) Load() (text string, conflict bool, err error) {
	bytes, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var record recoveryRecord
	if err = json.Unmarshal(bytes, &record); err != nil {
		return "", false, err
	}
	if record.Version != 1 {
		return "", false, os.ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadedBase = record.Baseline
	return record.Content, record.Base != r.base, nil
}
func (r *Recovery) Queue(text string) {
	r.mu.Lock()
	r.sequence++
	sequence := r.sequence
	generation := r.generation
	base := r.base
	baseText := r.baseText
	r.mu.Unlock()
	go func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if generation != r.generation || sequence != r.sequence {
			return
		}
		record := recoveryRecord{Version: 1, Base: base, Content: text, Baseline: baseText}
		bytes, err := json.Marshal(record)
		if err == nil {
			err = writePrivate(r.path, bytes)
		}
		r.err = err
	}()
}
func (r *Recovery) Clear(content string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generation++
	r.base = config.Revision([]byte(content))
	r.baseText = content
	err := os.Remove(r.path)
	if os.IsNotExist(err) {
		err = nil
	}
	r.err = err
	return err
}
func (r *Recovery) Err() error { r.mu.Lock(); defer r.mu.Unlock(); return r.err }
func writePrivate(path string, bytes []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".modmux-recovery-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(bytes); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (r *Recovery) Rebase(content string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	base := config.Revision([]byte(content))
	if base != r.base {
		r.base = base
		r.baseText = content
		r.generation++
	}
}

func (r *Recovery) Baseline() string { r.mu.Lock(); defer r.mu.Unlock(); return r.loadedBase }
