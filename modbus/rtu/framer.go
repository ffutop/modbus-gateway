// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtu

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ffutop/modbus-gateway/modbus"
)

var ErrRequestTimedOut = errors.New("modbus: request timed out")

const (
	stateSlaveID = 1 << iota
	stateFunctionCode
	stateReadLength
	stateReadPayload
	stateCRC
)

type InvalidLengthError struct {
	Length byte
}

func (e *InvalidLengthError) Error() string {
	return fmt.Sprintf("invalid length received: %d", e.Length)
}

// CalculateResponseLength returns the expected length of a response ADU.
func CalculateResponseLength(adu []byte) int {
	length := MinSize
	switch adu[1] {
	case modbus.FuncCodeReadDiscreteInputs,
		modbus.FuncCodeReadCoils:
		count := int(binary.BigEndian.Uint16(adu[4:]))
		length += 1 + count/8
		if count%8 != 0 {
			length++
		}
	case modbus.FuncCodeReadInputRegisters,
		modbus.FuncCodeReadHoldingRegisters,
		modbus.FuncCodeReadWriteMultipleRegisters:
		count := int(binary.BigEndian.Uint16(adu[4:]))
		length += 1 + count*2
	case modbus.FuncCodeWriteSingleCoil,
		modbus.FuncCodeWriteMultipleCoils,
		modbus.FuncCodeWriteSingleRegister,
		modbus.FuncCodeWriteMultipleRegisters:
		length += 4
	case modbus.FuncCodeMaskWriteRegister:
		length += 6
	case modbus.FuncCodeReadFIFOQueue,
		modbus.FuncCodeReadDeviceIdentification:
		// undetermined
	default:
	}
	return length
}

// CalculateRequestLength returns the expected total length of the Request RTU ADU based on the header.
func CalculateRequestLength(funcCode byte, header []byte) (int, error) {
	// Header should be at least 7 bytes to cover ByteCount for 0x0F/0x10.
	// [SlaveID, Func, Appd1, Appd2, Appd3, Appd4/ByteCount]

	switch funcCode {
	case FuncCodeReadCoils,
		FuncCodeReadDiscreteInputs,
		FuncCodeReadHoldingRegister,
		FuncCodeReadInputRegister,
		FuncCodeWriteSingleCoil,
		FuncCodeWriteSingleRegister:
		// Fixed 8 bytes: [SlaveID, Func, Addr(2), Val(2), CRC(2)]
		return 8, nil
	case FuncCodeWriteMultipleCoils,
		FuncCodeWriteMultipleRegister:
		// Write Multiple
		// Req: [SlaveID, Func, Addr(2), Quant(2), ByteCount(1), Data(N), CRC(2)]
		// ByteCount is at Offset 6 (0-indexed) = header[6]

		if len(header) < 7 {
			return 0, fmt.Errorf("need 7 bytes to determine length for 0x%02X, got %d", funcCode, len(header))
		}

		byteCount := int(header[6])
		// Total = 7 (Header up to ByteCount) + N (Data) + 2 (CRC)
		return 7 + byteCount + 2, nil
	default:
		// Assume unknown function codes are not supported or have fixed minimal length?
		// For robustness, discard.
		return 0, fmt.Errorf("unsupported function code: 0x%02X", funcCode)
	}
}

// ReadResponse reads an RTU frame incrementally from the reader.
// It uses a state machine to detect the frame based on the expected SlaveID and FunctionCode.
func ReadResponse(slaveID, functionCode byte, r io.Reader, deadline time.Time) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("reader is nil")
	}

	buf := make([]byte, 1)
	data := make([]byte, MaxSize)

	state := stateSlaveID
	var length, toRead byte
	var n, crcCount int

	for {
		if time.Now().After(deadline) {
			return nil, ErrRequestTimedOut
		}

		if _, err := io.ReadAtLeast(r, buf, 1); err != nil {
			return nil, err
		}

		switch state {
		case stateSlaveID:
			if buf[0] == slaveID {
				state = stateFunctionCode
				data[n] = buf[0]
				n++
				continue
			}
		case stateFunctionCode:
			if buf[0] == functionCode {
				switch functionCode {
				case FuncCodeReadDiscreteInputs,
					FuncCodeReadCoils,
					FuncCodeReadHoldingRegister,
					FuncCodeReadInputRegister,
					FuncCodeReadWriteMultipleRegister,
					FuncCodeReadFIFOQueue:

					state = stateReadLength
				case FuncCodeWriteSingleCoil,
					FuncCodeWriteSingleRegister,
					FuncCodeWriteMultipleRegister,
					FuncCodeWriteMultipleCoils:

					state = stateReadPayload
					toRead = 4
				case FuncCodeMaskWriteRegister:
					state = stateReadPayload
					toRead = 6
				default:
					return nil, fmt.Errorf("functioncode not handled: %d", functionCode)
				}
				data[n] = buf[0]
				n++
				continue
			} else if buf[0] == functionCode+0x80 {
				state = stateReadPayload
				data[n] = buf[0]
				n++
				toRead = 1
			}
		case stateReadLength:
			length = buf[0]
			if length > MaxSize-5 || length == 0 {
				return nil, &InvalidLengthError{Length: length}
			}
			toRead = length
			data[n] = length
			n++
			state = stateReadPayload
		case stateReadPayload:
			data[n] = buf[0]
			toRead--
			n++
			if toRead == 0 {
				state = stateCRC
			}
		case stateCRC:
			data[n] = buf[0]
			crcCount++
			n++
			if crcCount == 2 {
				return data[:n], nil
			}
		}
	}
}
