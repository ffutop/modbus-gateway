// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package persistence

import (
	"fmt"
	"log/slog"
	"os"
	"syscall"
	"unsafe"

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
	data []byte
}

const (
	sizeCoils    = model.MaxAddress + 1
	sizeDiscrete = model.MaxAddress + 1
	sizeHolding  = (model.MaxAddress + 1) * 2
	sizeInput    = (model.MaxAddress + 1) * 2
	totalSize    = sizeCoils + sizeDiscrete + sizeHolding + sizeInput

	offsetCoils    = 0
	offsetDiscrete = offsetCoils + sizeCoils
	offsetHolding  = offsetDiscrete + sizeDiscrete
	offsetInput    = offsetHolding + sizeHolding
)

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
	data, err := syscall.Mmap(int(f.Fd()), 0, totalSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("mmap failed: %w", err)
	}
	ms.data = data

	// Construct the DataModel backed by the mmap slice
	m := &model.DataModel{}

	// Coils (Bytes)
	m.Coils = data[offsetCoils : offsetCoils+sizeCoils]

	// Discrete Inputs (Bytes)
	m.DiscreteInputs = data[offsetDiscrete : offsetDiscrete+sizeDiscrete]

	// Holding Registers (Uint16)
	// We use unsafe to create a []uint16 slice backed by the byte array.
	// Note: This relies on host endianness (LittleEndian usually).
	// Modbus protocol handling in DataModel must handle endian conversions if necessary,
	// but currently DataModel stores uint16 and handles BigEndian conversion during Read/Write bytes.
	// So storing as host-uint16 is correct for in-memory manipulation.
	holdingBytes := data[offsetHolding : offsetHolding+sizeHolding]
	m.HoldingRegisters = unsafe.Slice((*uint16)(unsafe.Pointer(&holdingBytes[0])), sizeHolding/2)

	// Input Registers (Uint16)
	inputBytes := data[offsetInput : offsetInput+sizeInput]
	m.InputRegisters = unsafe.Slice((*uint16)(unsafe.Pointer(&inputBytes[0])), sizeInput/2)

	return m, nil
}

// Save flushes the mmap to disk.
func (ms *MmapStorage) Save(m *model.DataModel) error {
	// MS_SYNC: Request synchronous write
	return ms.sync()
}

// OnWrite triggers a sync for persistence.
func (ms *MmapStorage) OnWrite(table model.TableType, address, quantity uint16) {
	// For "Real-time" persistence, we call Msync.
	// If performance is an issue, we could rely on OS lazy writeback (MAP_SHARED)
	// and only Msync periodically or on critical events.
	// Given the requirement "ensure data can be recovered", we should sync.
	if err := ms.sync(); err != nil {
		slog.Error("Failed to sync mmap", "err", err)
	}
}

func (ms *MmapStorage) sync() error {
	if ms.data == nil {
		return nil
	}
	// SYS_MSYNC logic
	// In Go syscall, Mmap is typically unmapped with Munmap.
	// Syncing is done via Msync.
	_, _, errno := syscall.Syscall(syscall.SYS_MSYNC, uintptr(unsafe.Pointer(&ms.data[0])), uintptr(len(ms.data)), syscall.MS_SYNC)
	if errno != 0 {
		return errno
	}
	return nil
}

// Close unmaps and closes the file.
func (ms *MmapStorage) Close() error {
	if ms.data != nil {
		syscall.Munmap(ms.data)
		ms.data = nil
	}
	if ms.file != nil {
		ms.file.Close()
		ms.file = nil
	}
	return nil
}
