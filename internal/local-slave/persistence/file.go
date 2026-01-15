// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package persistence

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
)

// FileStorage implements persistence using file operations.
// This provides OS-managed persistence and efficient memory usage.
//
// Layout:
// - Coils: 65536 bytes (Offset 0)
// - DiscreteInputs: 65536 bytes (Offset 65536)
// - HoldingRegisters: 65536 * 2 bytes (Offset 131072)
// - InputRegisters: 65536 * 2 bytes (Offset 262144)
// Total Size: 393216 bytes
type FileStorage struct {
	path string
	file *os.File
	data []byte
}

// NewFileStorage creates a new FileStorage.
func NewFileStorage(path string) *FileStorage {
	return &FileStorage{
		path: path,
	}
}

// Load loads the data model by file operations.
func (ms *FileStorage) Load() (*model.DataModel, error) {
	// Open file, creating if necessary
	f, err := os.OpenFile(ms.path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	ms.file = f

	// Ensure file size
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if fi.Size() != int64(totalSize) {
		if err := f.Truncate(int64(totalSize)); err != nil {
			f.Close()
			return nil, fmt.Errorf("failed to resize file: %w", err)
		}
	}

	data, err := io.ReadAll(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	ms.data = data

	// Construct the DataModel backed by the file data slice
	return mapBytesToModel(data), nil
}

// Save flushes the data to disk.
func (ms *FileStorage) Save(m *model.DataModel) error {
	return ms.sync()
}

// OnWrite triggers a sync for persistence.
func (ms *FileStorage) OnWrite(table model.TableType, address, quantity uint16) {
	// For "Real-time" persistence, we sync the file.
	// Given the requirement "ensure data can be recovered", we should sync.
	if err := ms.sync(); err != nil {
		slog.Error("Failed to sync file", "err", err)
	}
}

func (ms *FileStorage) sync() error {
	if ms.data == nil || ms.file == nil {
		return nil
	}
	if _, err := ms.file.WriteAt(ms.data, 0); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	if err := ms.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file to disk: %w", err)
	}
	return nil
}

// Close the file.
func (ms *FileStorage) Close() error {
	ms.file.Close()
	return nil
}
