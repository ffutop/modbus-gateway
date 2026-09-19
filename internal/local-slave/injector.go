// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package localslave

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/modbus"
)

// ResolvedMapping is a validated, address-resolved mapping from a standard
// Modbus write source range (Coils or HoldingRegisters) to a target range
// (DiscreteInputs or InputRegisters, respectively) in a shared simulation.
type ResolvedMapping struct {
	SourceTable model.TableType // TableCoils or TableHoldingRegisters
	SourceStart uint16
	Count       uint16
	TargetTable model.TableType // TableDiscreteInputs or TableInputRegisters
	TargetStart uint16
}

// InjectorSlave implements the "Scheme 1" standard Modbus write mapping: it
// only accepts FC05/FC06/FC15/FC16 and translates writes landing fully
// inside one configured mapping into a write against the shared simulation's
// target table. It never modifies Coils/HoldingRegisters, and never serves
// reads.
type InjectorSlave struct {
	sim      *simulation.Simulation
	mappings []ResolvedMapping
}

// NewInjectorSlave creates an InjectorSlave backed by the given shared
// simulation and mappings. Mapping validity (table pairing, range bounds,
// non-overlap) is expected to already have been checked at config load time.
func NewInjectorSlave(sim *simulation.Simulation, mappings []ResolvedMapping) *InjectorSlave {
	return &InjectorSlave{sim: sim, mappings: mappings}
}

// Process executes the injector's Function Code against the shared
// simulation model.
func (s *InjectorSlave) Process(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	switch req.FunctionCode {
	case modbus.FuncCodeWriteSingleCoil:
		return s.handleWriteSingleCoil(ctx, req)
	case modbus.FuncCodeWriteMultipleCoils:
		return s.handleWriteMultipleCoils(ctx, req)
	case modbus.FuncCodeWriteSingleRegister:
		return s.handleWriteSingleRegister(ctx, req)
	case modbus.FuncCodeWriteMultipleRegisters:
		return s.handleWriteMultipleRegisters(ctx, req)
	default:
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalFunction), nil
	}
}

// findMapping returns the single mapping whose source table matches and
// whose source range fully contains [address, address+quantity), or nil if
// no such mapping exists (unmapped address, out of range, or crossing a
// mapping boundary).
func (s *InjectorSlave) findMapping(sourceTable model.TableType, address, quantity uint16) *ResolvedMapping {
	start := uint32(address)
	end := start + uint32(quantity)
	for i := range s.mappings {
		m := &s.mappings[i]
		if m.SourceTable != sourceTable {
			continue
		}
		mEnd := uint32(m.SourceStart) + uint32(m.Count)
		if start >= uint32(m.SourceStart) && end <= mEnd {
			return m
		}
	}
	return nil
}

func (s *InjectorSlave) handleWriteSingleCoil(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	m := s.findMapping(model.TableCoils, address, 1)
	if m == nil {
		auditWrite(ctx, s.sim, "injector", "discrete_inputs", address, 1, errUnmapped)
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	target := m.TargetStart + (address - m.SourceStart)

	err := s.sim.WriteSingleDiscreteInput(target, value)
	auditWrite(ctx, s.sim, "injector", "discrete_inputs", target, 1, err)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	return req, nil // Echo request
}

func (s *InjectorSlave) handleWriteMultipleCoils(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) < 6 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])
	byteCount := req.Data[4]

	if quantity < 1 || quantity > 1968 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	if byte(len(req.Data)-5) != byteCount {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	m := s.findMapping(model.TableCoils, address, quantity)
	if m == nil {
		auditWrite(ctx, s.sim, "injector", "discrete_inputs", address, quantity, errUnmapped)
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	target := m.TargetStart + (address - m.SourceStart)

	err := s.sim.WriteMultipleDiscreteInputs(target, quantity, req.Data[5:])
	auditWrite(ctx, s.sim, "injector", "discrete_inputs", target, quantity, err)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	return packWriteResponse(req.FunctionCode, address, quantity), nil
}

func (s *InjectorSlave) handleWriteSingleRegister(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	m := s.findMapping(model.TableHoldingRegisters, address, 1)
	if m == nil {
		auditWrite(ctx, s.sim, "injector", "input_registers", address, 1, errUnmapped)
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	target := m.TargetStart + (address - m.SourceStart)

	err := s.sim.WriteSingleInputRegister(target, value)
	auditWrite(ctx, s.sim, "injector", "input_registers", target, 1, err)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	return req, nil // Echo request
}

func (s *InjectorSlave) handleWriteMultipleRegisters(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) < 6 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])
	byteCount := req.Data[4]

	if quantity < 1 || quantity > 123 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	if byte(len(req.Data)-5) != byteCount {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	m := s.findMapping(model.TableHoldingRegisters, address, quantity)
	if m == nil {
		auditWrite(ctx, s.sim, "injector", "input_registers", address, quantity, errUnmapped)
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	target := m.TargetStart + (address - m.SourceStart)

	err := s.sim.WriteMultipleInputRegisters(target, quantity, req.Data[5:])
	auditWrite(ctx, s.sim, "injector", "input_registers", target, quantity, err)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	return packWriteResponse(req.FunctionCode, address, quantity), nil
}

var errUnmapped = fmt.Errorf("address range is not covered by any configured mapping")
