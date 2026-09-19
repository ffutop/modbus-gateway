// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package localslave

import (
	"context"
	"testing"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
	"github.com/ffutop/modbus-gateway/internal/local-slave/persistence"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/modbus"
)

func newTestInjector(t *testing.T, mappings []ResolvedMapping) (*InjectorSlave, *simulation.Simulation) {
	t.Helper()
	sim, err := simulation.Open("sim", persistence.NewMemoryStorage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return NewInjectorSlave(sim, mappings), sim
}

func discreteInputMapping() []ResolvedMapping {
	return []ResolvedMapping{
		{SourceTable: model.TableCoils, SourceStart: 0, Count: 16, TargetTable: model.TableDiscreteInputs, TargetStart: 100},
	}
}

func inputRegisterMapping() []ResolvedMapping {
	return []ResolvedMapping{
		{SourceTable: model.TableHoldingRegisters, SourceStart: 0, Count: 8, TargetTable: model.TableInputRegisters, TargetStart: 200},
	}
}

func TestInjectorSlave_WriteMultipleCoils_MapsToDiscreteInputs(t *testing.T) {
	s, sim := newTestInjector(t, discreteInputMapping())
	ctx := context.Background()

	// FC15: address=0, quantity=16, byteCount=2, data=0xFF,0x00 (first 8 ON, next 8 OFF)
	req := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteMultipleCoils,
		Data:         []byte{0x00, 0x00, 0x00, 0x10, 0x02, 0xFF, 0x00},
	}
	resp, err := s.Process(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FunctionCode&0x80 != 0 {
		t.Fatalf("expected success response, got exception %+v", resp)
	}

	data, err := sim.Model.ReadDiscreteInputs(100, 16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data[0] != 0xFF || data[1] != 0x00 {
		t.Errorf("expected target discrete inputs 100-115 to be 0xFF,0x00, got %x", data)
	}
}

func TestInjectorSlave_WriteSingleCoil_MapsToDiscreteInput(t *testing.T) {
	s, sim := newTestInjector(t, discreteInputMapping())
	ctx := context.Background()

	req := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteSingleCoil,
		Data:         []byte{0x00, 0x05, 0xFF, 0x00},
	}
	if _, err := s.Process(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := sim.Model.ReadDiscreteInputs(105, 1)
	if data[0] != 1 {
		t.Errorf("expected target discrete input 105 to be set")
	}
}

func TestInjectorSlave_WriteMultipleRegisters_MapsToInputRegisters(t *testing.T) {
	s, sim := newTestInjector(t, inputRegisterMapping())
	ctx := context.Background()

	// FC16: address=0, quantity=2, byteCount=4, data = 0x0001, 0x0002
	req := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteMultipleRegisters,
		Data:         []byte{0x00, 0x00, 0x00, 0x02, 0x04, 0x00, 0x01, 0x00, 0x02},
	}
	if _, err := s.Process(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := sim.Model.ReadInputRegisters(200, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data[1] != 0x01 || data[3] != 0x02 {
		t.Errorf("expected target input registers 200-201 to be 1,2, got %x", data)
	}
}

func TestInjectorSlave_WriteSingleRegister_MapsToInputRegister(t *testing.T) {
	s, sim := newTestInjector(t, inputRegisterMapping())
	ctx := context.Background()

	req := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteSingleRegister,
		Data:         []byte{0x00, 0x03, 0xBE, 0xEF},
	}
	if _, err := s.Process(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := sim.Model.ReadInputRegisters(203, 1)
	if data[0] != 0xBE || data[1] != 0xEF {
		t.Errorf("expected target input register 203 to be 0xBEEF, got %x", data)
	}
}

func TestInjectorSlave_UnmappedAddress_RejectedAndTargetUnchanged(t *testing.T) {
	s, sim := newTestInjector(t, discreteInputMapping())
	ctx := context.Background()

	req := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteSingleCoil,
		Data:         []byte{0x00, 0x14, 0xFF, 0x00}, // address 20, outside mapped [0,16)
	}
	resp, err := s.Process(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FunctionCode&0x80 == 0 || resp.Data[0] != modbus.ExceptionCodeIllegalDataAddress {
		t.Errorf("expected IllegalDataAddress exception, got %+v", resp)
	}
	if sim.Version() != 0 {
		t.Errorf("expected no commit for a rejected write, got version %d", sim.Version())
	}
}

func TestInjectorSlave_WriteCrossingMappingBoundary_Rejected(t *testing.T) {
	s, _ := newTestInjector(t, discreteInputMapping())
	ctx := context.Background()

	// mapping covers coils [0,16); request [10,26) crosses the boundary.
	req := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteMultipleCoils,
		Data:         []byte{0x00, 0x0A, 0x00, 0x10, 0x02, 0xFF, 0xFF},
	}
	resp, err := s.Process(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FunctionCode&0x80 == 0 || resp.Data[0] != modbus.ExceptionCodeIllegalDataAddress {
		t.Errorf("expected IllegalDataAddress exception, got %+v", resp)
	}
}

func TestInjectorSlave_IllegalFunctionCode_Rejected(t *testing.T) {
	s, _ := newTestInjector(t, discreteInputMapping())
	ctx := context.Background()

	// FC01 (read coils) is not a valid injector request.
	req := modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeReadCoils,
		Data:         []byte{0x00, 0x00, 0x00, 0x01},
	}
	resp, err := s.Process(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FunctionCode != modbus.FuncCodeReadCoils|0x80 || resp.Data[0] != modbus.ExceptionCodeIllegalFunction {
		t.Errorf("expected IllegalFunction exception, got %+v", resp)
	}
}

func TestInjectorSlave_MultipleNonOverlappingMappings(t *testing.T) {
	mappings := append(discreteInputMapping(), inputRegisterMapping()...)
	s, sim := newTestInjector(t, mappings)
	ctx := context.Background()

	if _, err := s.Process(ctx, modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteSingleCoil,
		Data:         []byte{0x00, 0x00, 0xFF, 0x00},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := s.Process(ctx, modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteSingleRegister,
		Data:         []byte{0x00, 0x00, 0x00, 0x2A},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	di, _ := sim.Model.ReadDiscreteInputs(100, 1)
	ir, _ := sim.Model.ReadInputRegisters(200, 1)
	if di[0] != 1 {
		t.Errorf("expected discrete input 100 set")
	}
	if ir[0] != 0 || ir[1] != 0x2A {
		t.Errorf("expected input register 200 to be 42, got %x", ir)
	}
}
