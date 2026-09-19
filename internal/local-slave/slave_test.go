// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package localslave

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/ffutop/modbus-gateway/internal/local-slave/persistence"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/modbus"
)

func newTestSlave(t *testing.T) (*LocalSlave, *simulation.Simulation) {
	t.Helper()
	sim, err := simulation.Open("sim", persistence.NewMemoryStorage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return NewLocalSlave(sim), sim
}

func TestLocalSlave_WriteAndReadCoil(t *testing.T) {
	s, sim := newTestSlave(t)
	ctx := context.Background()

	writeReq := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteSingleCoil,
		Data:         []byte{0x00, 0x05, 0xFF, 0x00},
	}
	if _, err := s.Process(ctx, writeReq); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sim.Version() != 1 {
		t.Errorf("expected version 1 after write, got %d", sim.Version())
	}

	readReq := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeReadCoils,
		Data:         []byte{0x00, 0x05, 0x00, 0x01},
	}
	resp, err := s.Process(ctx, readReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data[1] != 0x01 {
		t.Errorf("expected coil 5 to read as set, got %x", resp.Data)
	}
}

func TestLocalSlave_WriteAndReadHoldingRegister(t *testing.T) {
	s, _ := newTestSlave(t)
	ctx := context.Background()

	writeReq := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteSingleRegister,
		Data:         []byte{0x00, 0x0A, 0x30, 0x39},
	}
	if _, err := s.Process(ctx, writeReq); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	readReq := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeReadHoldingRegisters,
		Data:         []byte{0x00, 0x0A, 0x00, 0x01},
	}
	resp, err := s.Process(ctx, readReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := binary.BigEndian.Uint16(resp.Data[1:3]); got != 0x3039 {
		t.Errorf("expected 0x3039, got 0x%X", got)
	}
}

func TestLocalSlave_ReadDiscreteInputs_ReflectsInjectedData(t *testing.T) {
	s, sim := newTestSlave(t)
	ctx := context.Background()

	// A business LocalSlave cannot itself write discrete inputs / input
	// registers, but must be able to read whatever the shared Simulation
	// model holds (e.g. written by an injector referencing the same model).
	if err := sim.WriteSingleDiscreteInput(2, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	readReq := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeReadDiscreteInputs,
		Data:         []byte{0x00, 0x02, 0x00, 0x01},
	}
	resp, err := s.Process(ctx, readReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data[1] != 0x01 {
		t.Errorf("expected discrete input 2 to read as set, got %x", resp.Data)
	}
}

func TestLocalSlave_UnknownFunctionCode_ReturnsIllegalFunction(t *testing.T) {
	s, _ := newTestSlave(t)
	ctx := context.Background()

	resp, err := s.Process(ctx, modbus.ProtocolDataUnit{FunctionCode: 0x63})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FunctionCode != 0x63|0x80 || resp.Data[0] != modbus.ExceptionCodeIllegalFunction {
		t.Errorf("expected illegal function exception, got %+v", resp)
	}
}
