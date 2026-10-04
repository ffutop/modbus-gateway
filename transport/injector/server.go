// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package injector

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/ffutop/modbus-gateway/internal/simulation/model"
	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/transport"
)

// mapping is a validated, address-resolved mapping from a standard
// Modbus write source range (Coils or HoldingRegisters) to a target range
// (DiscreteInputs or InputRegisters, respectively) in a shared simulation.
type mapping struct {
	SourceTable model.TableType // TableCoils or TableHoldingRegisters
	SourceStart uint16
	Count       uint16
	TargetTable model.TableType // TableDiscreteInputs or TableInputRegisters
	TargetStart uint16
}

// Send maps the request's write into the shared simulation model; slaveID
// is already routed to this downstream.
func (c *Client) Send(ctx context.Context, slaveID byte, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	switch req.FunctionCode {
	case modbus.FuncCodeWriteSingleCoil:
		return c.handleWriteSingleCoil(ctx, req)
	case modbus.FuncCodeWriteMultipleCoils:
		return c.handleWriteMultipleCoils(ctx, req)
	case modbus.FuncCodeWriteSingleRegister:
		return c.handleWriteSingleRegister(ctx, req)
	case modbus.FuncCodeWriteMultipleRegisters:
		return c.handleWriteMultipleRegisters(ctx, req)
	default:
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalFunction), nil
	}
}

// findMapping returns the single mapping whose source table matches and
// whose source range fully contains [address, address+quantity), or nil if
// no such mapping exists (unmapped address, out of range, or crossing a
// mapping boundary).
func (c *Client) findMapping(sourceTable model.TableType, address, quantity uint16) *mapping {
	start := uint32(address)
	end := start + uint32(quantity)
	for i := range c.mappings {
		m := &c.mappings[i]
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

func (c *Client) handleWriteSingleCoil(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	m := c.findMapping(model.TableCoils, address, 1)
	if m == nil {
		c.sim.Audit(transport.SourceAddr(ctx), "injector", "discrete_inputs", address, 1, errUnmapped)
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	target := m.TargetStart + (address - m.SourceStart)

	err := c.sim.WriteSingleDiscreteInput(target, value)
	c.sim.Audit(transport.SourceAddr(ctx), "injector", "discrete_inputs", target, 1, err)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	return req, nil // Echo request
}

func (c *Client) handleWriteMultipleCoils(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) < 6 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])
	byteCount := req.Data[4]

	if quantity < 1 || quantity > 1968 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	if byte(len(req.Data)-5) != byteCount {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	m := c.findMapping(model.TableCoils, address, quantity)
	if m == nil {
		c.sim.Audit(transport.SourceAddr(ctx), "injector", "discrete_inputs", address, quantity, errUnmapped)
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	target := m.TargetStart + (address - m.SourceStart)

	err := c.sim.WriteMultipleDiscreteInputs(target, quantity, req.Data[5:])
	c.sim.Audit(transport.SourceAddr(ctx), "injector", "discrete_inputs", target, quantity, err)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	return modbus.NewWriteResponse(req.FunctionCode, address, quantity), nil
}

func (c *Client) handleWriteSingleRegister(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	m := c.findMapping(model.TableHoldingRegisters, address, 1)
	if m == nil {
		c.sim.Audit(transport.SourceAddr(ctx), "injector", "input_registers", address, 1, errUnmapped)
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	target := m.TargetStart + (address - m.SourceStart)

	err := c.sim.WriteSingleInputRegister(target, value)
	c.sim.Audit(transport.SourceAddr(ctx), "injector", "input_registers", target, 1, err)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	return req, nil // Echo request
}

func (c *Client) handleWriteMultipleRegisters(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) < 6 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])
	byteCount := req.Data[4]

	if quantity < 1 || quantity > 123 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	if byte(len(req.Data)-5) != byteCount {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	m := c.findMapping(model.TableHoldingRegisters, address, quantity)
	if m == nil {
		c.sim.Audit(transport.SourceAddr(ctx), "injector", "input_registers", address, quantity, errUnmapped)
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	target := m.TargetStart + (address - m.SourceStart)

	err := c.sim.WriteMultipleInputRegisters(target, quantity, req.Data[5:])
	c.sim.Audit(transport.SourceAddr(ctx), "injector", "input_registers", target, quantity, err)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}
	return modbus.NewWriteResponse(req.FunctionCode, address, quantity), nil
}

var errUnmapped = fmt.Errorf("address range is not covered by any configured mapping")
