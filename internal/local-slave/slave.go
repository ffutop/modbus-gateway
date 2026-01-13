// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package localslave

import (
	"encoding/binary"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
	"github.com/ffutop/modbus-gateway/modbus"
)

// LocalSlave implements the Modbus protocol logic on top of a DataModel.
type LocalSlave struct {
	model *model.DataModel
}

// NewLocalSlave creates a new LocalSlave.
func NewLocalSlave(m *model.DataModel) *LocalSlave {
	return &LocalSlave{model: m}
}

// Process executes the Modbus Function Code against the memory model.
func (s *LocalSlave) Process(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
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
		return s.handleWriteSingleCoil(req)
	case modbus.FuncCodeWriteSingleRegister:
		return s.handleWriteSingleRegister(req)
	case modbus.FuncCodeWriteMultipleCoils:
		return s.handleWriteMultipleCoils(req)
	case modbus.FuncCodeWriteMultipleRegisters:
		return s.handleWriteMultipleRegisters(req)
	default:
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalFunction), nil
	}
}

func (s *LocalSlave) handleReadCoils(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 2000 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := s.model.ReadCoils(address, quantity)
	if err != nil {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	respData := make([]byte, 1+len(data))
	respData[0] = byte(len(data))
	copy(respData[1:], data)

	return modbus.ProtocolDataUnit{
		FunctionCode: req.FunctionCode,
		Data:         respData,
	}, nil
}

func (s *LocalSlave) handleReadDiscreteInputs(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 2000 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := s.model.ReadDiscreteInputs(address, quantity)
	if err != nil {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	respData := make([]byte, 1+len(data))
	respData[0] = byte(len(data))
	copy(respData[1:], data)

	return modbus.ProtocolDataUnit{
		FunctionCode: req.FunctionCode,
		Data:         respData,
	}, nil
}

func (s *LocalSlave) handleReadHoldingRegisters(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 125 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := s.model.ReadHoldingRegisters(address, quantity)
	if err != nil {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	respData := make([]byte, 1+len(data))
	respData[0] = byte(len(data))
	copy(respData[1:], data)

	return modbus.ProtocolDataUnit{
		FunctionCode: req.FunctionCode,
		Data:         respData,
	}, nil
}

func (s *LocalSlave) handleReadInputRegisters(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])

	if quantity < 1 || quantity > 125 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	data, err := s.model.ReadInputRegisters(address, quantity)
	if err != nil {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	respData := make([]byte, 1+len(data))
	respData[0] = byte(len(data))
	copy(respData[1:], data)

	return modbus.ProtocolDataUnit{
		FunctionCode: req.FunctionCode,
		Data:         respData,
	}, nil
}

func (s *LocalSlave) handleWriteSingleCoil(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	if err := s.model.WriteSingleCoil(address, value); err != nil {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return req, nil // Echo request
}

func (s *LocalSlave) handleWriteSingleRegister(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) != 4 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	value := binary.BigEndian.Uint16(req.Data[2:4])

	if err := s.model.WriteSingleRegister(address, value); err != nil {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	return req, nil // Echo request
}

func (s *LocalSlave) handleWriteMultipleCoils(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) < 6 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])
	byteCount := req.Data[4]

	if quantity < 1 || quantity > 1968 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	if byte(len(req.Data)-5) != byteCount {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	if err := s.model.WriteMultipleCoils(address, quantity, req.Data[5:]); err != nil {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	respData := make([]byte, 4)
	binary.BigEndian.PutUint16(respData[0:2], address)
	binary.BigEndian.PutUint16(respData[2:4], quantity)

	return modbus.ProtocolDataUnit{
		FunctionCode: req.FunctionCode,
		Data:         respData,
	}, nil
}

func (s *LocalSlave) handleWriteMultipleRegisters(req modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	if len(req.Data) < 6 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}
	address := binary.BigEndian.Uint16(req.Data[0:2])
	quantity := binary.BigEndian.Uint16(req.Data[2:4])
	byteCount := req.Data[4]

	if quantity < 1 || quantity > 123 {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	if byte(len(req.Data)-5) != byteCount {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataValue), nil
	}

	if err := s.model.WriteMultipleRegisters(address, quantity, req.Data[5:]); err != nil {
		return s.exception(req.FunctionCode, modbus.ExceptionCodeIllegalDataAddress), nil
	}

	respData := make([]byte, 4)
	binary.BigEndian.PutUint16(respData[0:2], address)
	binary.BigEndian.PutUint16(respData[2:4], quantity)

	return modbus.ProtocolDataUnit{
		FunctionCode: req.FunctionCode,
		Data:         respData,
	}, nil
}

func (s *LocalSlave) exception(funcCode byte, code byte) modbus.ProtocolDataUnit {
	return modbus.ProtocolDataUnit{
		FunctionCode: funcCode | 0x80,
		Data:         []byte{code},
	}
}
