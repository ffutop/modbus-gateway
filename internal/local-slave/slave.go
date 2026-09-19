// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package localslave

import (
	"context"
	"encoding/binary"

	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/modbus"
)

// LocalSlave implements the standard Modbus protocol logic (FC01-06,15,16)
// against a shared simulation model. Multiple LocalSlaves (and Injectors)
// may reference the same *simulation.Simulation.
type LocalSlave struct {
	sim *simulation.Simulation
}

// NewLocalSlave creates a new LocalSlave backed by the given shared simulation.
func NewLocalSlave(sim *simulation.Simulation) *LocalSlave {
	return &LocalSlave{sim: sim}
}

// Process executes the Modbus Function Code against the shared simulation model.
func (s *LocalSlave) Process(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	switch req.FunctionCode {
	case modbus.FuncCodeReadCoils:
		return s.handleReadCoils(req)
	case modbus.FuncCodeReadDiscreteInputs:
		return s.handleReadDiscreteInputs(req)
	case modbus.FuncCodeReadHoldingRegisters:
		return s.handleReadHoldingRegisters(req)
	case modbus.FuncCodeReadInputRegisters:
		return s.handleReadInputRegisters(req)
	case modbus.FuncCodeWriteSingleCoil:
		return s.handleWriteSingleCoil(ctx, req)
	case modbus.FuncCodeWriteSingleRegister:
		return s.handleWriteSingleRegister(ctx, req)
	case modbus.FuncCodeWriteMultipleCoils:
		return s.handleWriteMultipleCoils(ctx, req)
	case modbus.FuncCodeWriteMultipleRegisters:
		return s.handleWriteMultipleRegisters(ctx, req)
	default:
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalFunction), nil
	}
}

func (s *LocalSlave) handleReadCoils(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 2000 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := s.sim.Model.ReadCoils(address, quantity)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return packReadResponse(req.FunctionCode, data), nil
}

func (s *LocalSlave) handleReadDiscreteInputs(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 2000 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := s.sim.Model.ReadDiscreteInputs(address, quantity)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return packReadResponse(req.FunctionCode, data), nil
}

func (s *LocalSlave) handleReadHoldingRegisters(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 125 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := s.sim.Model.ReadHoldingRegisters(address, quantity)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return packReadResponse(req.FunctionCode, data), nil
}

func (s *LocalSlave) handleReadInputRegisters(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 125 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := s.sim.Model.ReadInputRegisters(address, quantity)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return packReadResponse(req.FunctionCode, data), nil
}

func (s *LocalSlave) handleWriteSingleCoil(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	err := s.sim.WriteSingleCoil(address, value)
	auditWrite(ctx, s.sim, "local", "coils", address, 1, err)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return req, nil // Echo request
}

func (s *LocalSlave) handleWriteSingleRegister(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	err := s.sim.WriteSingleRegister(address, value)
	auditWrite(ctx, s.sim, "local", "holding_registers", address, 1, err)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return req, nil // Echo request
}

func (s *LocalSlave) handleWriteMultipleCoils(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
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

	err := s.sim.WriteMultipleCoils(address, quantity, req.Data[5:])
	auditWrite(ctx, s.sim, "local", "coils", address, quantity, err)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return packWriteResponse(req.FunctionCode, address, quantity), nil
}

func (s *LocalSlave) handleWriteMultipleRegisters(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
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

	err := s.sim.WriteMultipleRegisters(address, quantity, req.Data[5:])
	auditWrite(ctx, s.sim, "local", "holding_registers", address, quantity, err)
	if err != nil {
		return exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return packWriteResponse(req.FunctionCode, address, quantity), nil
}
