// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package local

import (
	"context"
	"encoding/binary"

	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/transport"
)

// Send executes the request's function code against the shared simulation
// model; slaveID is already routed to this downstream.
func (c *Client) Send(ctx context.Context, slaveID byte, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	switch req.FunctionCode {
	case modbus.FuncCodeReadCoils:
		return c.handleReadCoils(req)
	case modbus.FuncCodeReadDiscreteInputs:
		return c.handleReadDiscreteInputs(req)
	case modbus.FuncCodeReadHoldingRegisters:
		return c.handleReadHoldingRegisters(req)
	case modbus.FuncCodeReadInputRegisters:
		return c.handleReadInputRegisters(req)
	case modbus.FuncCodeWriteSingleCoil:
		return c.handleWriteSingleCoil(ctx, req)
	case modbus.FuncCodeWriteSingleRegister:
		return c.handleWriteSingleRegister(ctx, req)
	case modbus.FuncCodeWriteMultipleCoils:
		return c.handleWriteMultipleCoils(ctx, req)
	case modbus.FuncCodeWriteMultipleRegisters:
		return c.handleWriteMultipleRegisters(ctx, req)
	default:
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalFunction), nil
	}
}

func (c *Client) handleReadCoils(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 2000 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := c.sim.Model.ReadCoils(address, quantity)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return modbus.NewReadResponse(req.FunctionCode, data), nil
}

func (c *Client) handleReadDiscreteInputs(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 2000 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := c.sim.Model.ReadDiscreteInputs(address, quantity)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return modbus.NewReadResponse(req.FunctionCode, data), nil
}

func (c *Client) handleReadHoldingRegisters(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 125 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := c.sim.Model.ReadHoldingRegisters(address, quantity)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return modbus.NewReadResponse(req.FunctionCode, data), nil
}

func (c *Client) handleReadInputRegisters(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 125 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := c.sim.Model.ReadInputRegisters(address, quantity)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return modbus.NewReadResponse(req.FunctionCode, data), nil
}

func (c *Client) handleWriteSingleCoil(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	err := c.sim.WriteSingleCoil(address, value)
	c.sim.Audit(transport.SourceAddr(ctx), "local", "coils", address, 1, err)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return req, nil // Echo request
}

func (c *Client) handleWriteSingleRegister(ctx context.Context, req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	err := c.sim.WriteSingleRegister(address, value)
	c.sim.Audit(transport.SourceAddr(ctx), "local", "holding_registers", address, 1, err)
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

	err := c.sim.WriteMultipleCoils(address, quantity, req.Data[5:])
	c.sim.Audit(transport.SourceAddr(ctx), "local", "coils", address, quantity, err)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return modbus.NewWriteResponse(req.FunctionCode, address, quantity), nil
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

	err := c.sim.WriteMultipleRegisters(address, quantity, req.Data[5:])
	c.sim.Audit(transport.SourceAddr(ctx), "local", "holding_registers", address, quantity, err)
	if err != nil {
		return modbus.NewException(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return modbus.NewWriteResponse(req.FunctionCode, address, quantity), nil
}
