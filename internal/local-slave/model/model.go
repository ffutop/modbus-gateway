// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package model

import (
	"encoding/binary"
	"fmt"
	"sync"
)

const (
	MaxAddress = 65535
)

// TableType represents the type of Modbus data table.
type TableType int

const (
	TableCoils TableType = iota
	TableDiscreteInputs
	TableHoldingRegisters
	TableInputRegisters
)

// DataModel holds the modbus data in memory.
// It uses a simple flat memory model covering the full 16-bit address space.
type DataModel struct {
	mu sync.RWMutex

	// 0x Coils (Read/Write). Stored as 1 (ON) or 0 (OFF).
	Coils []byte
	// 1x Discrete Inputs (Read Only). Stored as 1 (ON) or 0 (OFF).
	DiscreteInputs []byte
	// 4x Holding Registers (Read/Write).
	HoldingRegisters []uint16
	// 3x Input Registers (Read Only).
	InputRegisters []uint16
}

// NewDataModel creates a new memory model initialized to zero.
func NewDataModel() *DataModel {
	return &DataModel{
		Coils:            make([]byte, MaxAddress+1),
		DiscreteInputs:   make([]byte, MaxAddress+1),
		HoldingRegisters: make([]uint16, MaxAddress+1),
		InputRegisters:   make([]uint16, MaxAddress+1),
	}
}

// ReadCoils reads a range of coils and returns them as packed bytes (Modbus format).
func (m *DataModel) ReadCoils(address, quantity uint16) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := validateRange(address, quantity); err != nil {
		return nil, err
	}

	// Calculate byte count: (quantity + 7) / 8
	byteCount := (int(quantity) + 7) / 8
	result := make([]byte, byteCount)

	for i := 0; i < int(quantity); i++ {
		if m.Coils[int(address)+i] != 0 {
			byteIdx := i / 8
			bitIdx := uint(i % 8)
			result[byteIdx] |= 1 << bitIdx
		}
	}

	return result, nil
}

// WriteSingleCoil writes a single coil. value should be 0xFF00 (ON) or 0x0000 (OFF).
func (m *DataModel) WriteSingleCoil(address uint16, value uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if int(address) > MaxAddress {
		return fmt.Errorf("address out of range")
	}

	switch value {
	case 0xFF00:
		m.Coils[address] = 1
	case 0x0000:
		m.Coils[address] = 0
	default:
		// Strictly speaking Modbus only allows these two, and we can just ignore others or error.
	}
	return nil
}

// WriteMultipleCoils writes a range of coils from packed bytes.
func (m *DataModel) WriteMultipleCoils(address, quantity uint16, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := validateRange(address, quantity); err != nil {
		return err
	}

	expectedBytes := (int(quantity) + 7) / 8
	if len(data) < expectedBytes {
		return fmt.Errorf("insufficient data length")
	}

	for i := 0; i < int(quantity); i++ {
		byteIdx := i / 8
		bitIdx := uint(i % 8)
		val := (data[byteIdx] >> bitIdx) & 1
		m.Coils[int(address)+i] = val
	}
	return nil
}

// ReadDiscreteInputs reads a range of discrete inputs and returns them as packed bytes.
func (m *DataModel) ReadDiscreteInputs(address, quantity uint16) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := validateRange(address, quantity); err != nil {
		return nil, err
	}

	byteCount := (int(quantity) + 7) / 8
	result := make([]byte, byteCount)

	for i := 0; i < int(quantity); i++ {
		if m.DiscreteInputs[int(address)+i] != 0 {
			byteIdx := i / 8
			bitIdx := uint(i % 8)
			result[byteIdx] |= 1 << bitIdx
		}
	}
	return result, nil
}

// ReadHoldingRegisters reads a range of holding registers and returns them as BigEndian bytes.
func (m *DataModel) ReadHoldingRegisters(address, quantity uint16) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := validateRange(address, quantity); err != nil {
		return nil, err
	}

	result := make([]byte, quantity*2)
	for i := 0; i < int(quantity); i++ {
		val := m.HoldingRegisters[int(address)+i]
		binary.BigEndian.PutUint16(result[i*2:], val)
	}
	return result, nil
}

// WriteSingleRegister writes a single holding register.
func (m *DataModel) WriteSingleRegister(address uint16, value uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if int(address) > MaxAddress {
		return fmt.Errorf("address out of range")
	}

	m.HoldingRegisters[address] = value
	return nil
}

// WriteMultipleRegisters writes a range of holding registers from BigEndian bytes.
func (m *DataModel) WriteMultipleRegisters(address, quantity uint16, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := validateRange(address, quantity); err != nil {
		return err
	}

	if len(data) < int(quantity)*2 {
		return fmt.Errorf("insufficient data length")
	}

	for i := 0; i < int(quantity); i++ {
		val := binary.BigEndian.Uint16(data[i*2:])
		m.HoldingRegisters[int(address)+i] = val
	}
	return nil
}

// ReadInputRegisters reads a range of input registers and returns them as BigEndian bytes.
func (m *DataModel) ReadInputRegisters(address, quantity uint16) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := validateRange(address, quantity); err != nil {
		return nil, err
	}

	result := make([]byte, quantity*2)
	for i := 0; i < int(quantity); i++ {
		val := m.InputRegisters[int(address)+i]
		binary.BigEndian.PutUint16(result[i*2:], val)
	}
	return result, nil
}

func validateRange(address, quantity uint16) error {
	if quantity == 0 {
		return fmt.Errorf("quantity must be greater than 0")
	}
	// address is 0-based.
	if int(address)+int(quantity) > MaxAddress+1 {
		return fmt.Errorf("address range out of bounds")
	}
	return nil
}
