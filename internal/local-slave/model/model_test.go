// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package model

import (
	"testing"
)

func TestWriteSingleDiscreteInput(t *testing.T) {
	m := NewDataModel()

	if err := m.WriteSingleDiscreteInput(5, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := m.ReadDiscreteInputs(5, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data[0] != 1 {
		t.Errorf("expected discrete input 5 to be 1, got %v", data[0])
	}

	if err := m.WriteSingleDiscreteInput(5, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ = m.ReadDiscreteInputs(5, 1)
	if data[0] != 0 {
		t.Errorf("expected discrete input 5 to be 0, got %v", data[0])
	}
}

func TestWriteMultipleDiscreteInputs(t *testing.T) {
	m := NewDataModel()

	// Pack 10 bits: 0b11 0000 0101 (LSB first within byte)
	// bits: idx0=1, idx1=0, idx2=1, idx3=0, idx4=0, idx5=0, idx6=0, idx7=1, idx8=1, idx9=0
	data := []byte{0b10000101, 0b00000001}
	if err := m.WriteMultipleDiscreteInputs(0, 10, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	read, err := m.ReadDiscreteInputs(0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(read) != 2 || read[0] != data[0] || read[1] != (data[1]&0x03) {
		t.Errorf("round-trip mismatch: wrote %08b %08b got %08b %08b", data[0], data[1], read[0], read[1])
	}

	if err := m.WriteMultipleDiscreteInputs(MaxAddress, 2, []byte{0x01}); err == nil {
		t.Error("expected error for range out of bounds")
	}

	if err := m.WriteMultipleDiscreteInputs(0, 10, []byte{0x00}); err == nil {
		t.Error("expected error for insufficient data length")
	}
}

func TestWriteSingleInputRegister(t *testing.T) {
	m := NewDataModel()

	if err := m.WriteSingleInputRegister(20, 0xBEEF); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := m.ReadInputRegisters(20, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 2 || data[0] != 0xBE || data[1] != 0xEF {
		t.Errorf("expected 0xBEEF, got %x", data)
	}
}

func TestWriteMultipleInputRegisters(t *testing.T) {
	m := NewDataModel()

	data := []byte{0x00, 0x01, 0x00, 0x02, 0x00, 0x03}
	if err := m.WriteMultipleInputRegisters(0, 3, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	read, err := m.ReadInputRegisters(0, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := range data {
		if read[i] != data[i] {
			t.Errorf("round-trip mismatch at byte %d: wrote %x got %x", i, data[i], read[i])
		}
	}

	if err := m.WriteMultipleInputRegisters(MaxAddress, 2, make([]byte, 4)); err == nil {
		t.Error("expected error for range out of bounds")
	}

	if err := m.WriteMultipleInputRegisters(0, 3, make([]byte, 4)); err == nil {
		t.Error("expected error for insufficient data length")
	}
}
