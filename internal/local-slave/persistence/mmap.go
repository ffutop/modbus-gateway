// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package persistence

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/edsrzf/mmap-go"
	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
)

// MmapStorage implements persistence using memory-mapped files.
// This provides OS-managed persistence and efficient memory usage.
//
// Layout:
// - Coils: 65536 bytes (Offset 0)
// - DiscreteInputs: 65536 bytes (Offset 65536)
// - HoldingRegisters: 65536 * 2 bytes (Offset 131072)
// - InputRegisters: 65536 * 2 bytes (Offset 262144)
// Total Size: 393216 bytes
type MmapStorage struct {
	path string
	file *os.File
	data mmap.MMap
}

// NewMmapStorage creates a new MmapStorage.
func NewMmapStorage(path string) *MmapStorage {
	return &MmapStorage{
		path: path,
	}
}

// Load loads the data model by memory-mapping the file.
func (ms *MmapStorage) Load() (*model.DataModel, error) {
	// Open file, creating if necessary
	f, err := os.OpenFile(ms.path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open mmap file: %w", err)
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
			return nil, fmt.Errorf("failed to resize mmap file: %w", err)
		}
	}

	// Mmap the file
	data, err := mmap.Map(f, mmap.RDWR, 0)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("mmap failed: %w", err)
	}
	ms.data = data

	// Construct the DataModel backed by the mmap slice
	return mapBytesToModel(data), nil
}

// Save flushes the mmap to disk.
func (ms *MmapStorage) Save(m *model.DataModel) error {
	if ms.data == nil {
		return fmt.Errorf("mmap data is nil")
	}
	return ms.data.Flush()
}

// OnWrite triggers a flush for persistence.
func (ms *MmapStorage) OnWrite(table model.TableType, address, quantity uint16) {
	if ms.data == nil {
		return
	}
	// For "Real-time" persistence, flush mmap data to disk
	if err := ms.data.Flush(); err != nil {
		slog.Error("Failed to flush mmap", "err", err)
	}
}

// Close unmaps and closes the file.
func (ms *MmapStorage) Close() error {
	var err error
	if ms.data != nil {
		if e := ms.data.Unmap(); e != nil {
			err = e
		}
		ms.data = nil
	}
	if ms.file != nil {
		if e := ms.file.Close(); e != nil {
			err = e
		}
		ms.file = nil
	}
	return err
}
