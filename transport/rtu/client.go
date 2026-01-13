// Copyright (c) 2014 Quoc-Viet Nguyen. All rights reserved.
// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtu

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/modbus/crc"
)

// ErrRequestTimedOut is returned when a response is not received within the specified timeout.
var ErrRequestTimedOut = errors.New("modbus: request timed out")

const (
	stateSlaveID = 1 << iota
	stateFunctionCode
	stateReadLength
	stateReadPayload
	stateCRC
)

const (
	readCoilsFunctionCode           = 0x01
	readDiscreteInputsFunctionCode  = 0x02
	readHoldingRegisterFunctionCode = 0x03
	readInputRegisterFunctionCode   = 0x04

	writeSingleCoilFunctionCode       = 0x05
	writeSingleRegisterFunctionCode   = 0x06
	writeMultipleRegisterFunctionCode = 0x10
	writeMultipleCoilsFunctionCode    = 0x0F
	maskWriteRegisterFunctionCode     = 0x16

	readWriteMultipleRegisterFunctionCode = 0x17
	readFifoQueueFunctionCode             = 0x18
)

// Client implements Downstream interface (Modbus RTU Master).
type Client struct {
	rtuSerialTransporter
}

// NewClient allocates and initializes a RTU Client.
func NewClient(cfg config.SerialConfig) *Client {
	client := &Client{}

	// Map internal config to serial.Config
	client.serialPort.Config.Address = cfg.Device
	client.serialPort.Config.BaudRate = cfg.BaudRate
	client.serialPort.Config.DataBits = cfg.DataBits
	client.serialPort.Config.StopBits = cfg.StopBits
	client.serialPort.Config.Parity = cfg.Parity
	client.serialPort.Config.Timeout = cfg.Timeout

	client.IdleTimeout = serialIdleTimeout
	return client
}

// Send sends a PDU to the Downstream Slave
func (mb *Client) Send(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	// Wrap PDU into RTU ADU: SlaveID(1) + Func(1) + Data + CRC(2)
	length := len(pdu.Data) + 4
	if length > rtuMaxSize {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("modbus: length of data '%v' must not be bigger than '%v'", length, rtuMaxSize)
	}

	aduBytes := make([]byte, length)
	aduBytes[0] = slaveID
	aduBytes[1] = pdu.FunctionCode
	copy(aduBytes[2:], pdu.Data)

	// Calculate CRC
	var c crc.CRC
	c.Reset().PushBytes(aduBytes[0 : length-2])
	checksum := c.Value()
	aduBytes[length-1] = byte(checksum >> 8)
	aduBytes[length-2] = byte(checksum)

	// Send via Serial
	respBytes, err := mb.rtuSerialTransporter.Send(ctx, aduBytes)
	if err != nil {
		return modbus.ProtocolDataUnit{}, err
	}

	// Check CRC
	respLen := len(respBytes)
	if respLen < rtuMinSize {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("response too short")
	}
	c.Reset().PushBytes(respBytes[0 : respLen-2])
	if checksum := uint16(respBytes[respLen-1])<<8 | uint16(respBytes[respLen-2]); checksum != c.Value() {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("modbus: response crc '%v' does not match expected '%v'", checksum, c.Value())
	}

	// Extract PDU
	return modbus.ProtocolDataUnit{
		FunctionCode: respBytes[1],
		Data:         respBytes[2 : respLen-2],
	}, nil
}

// rtuSerialTransporter implements underlying serial comms.
type rtuSerialTransporter struct {
	serialPort
}

type InvalidLengthError struct {
	length byte
}

func (e *InvalidLengthError) Error() string {
	return fmt.Sprintf("invalid length received: %d", e.length)
}

// readIncrementally... (same)
func readIncrementally(slaveID, functionCode byte, r io.Reader, deadline time.Time) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("reader is nil")
	}

	buf := make([]byte, 1)
	data := make([]byte, rtuMaxSize)

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
				case readDiscreteInputsFunctionCode,
					readCoilsFunctionCode,
					readHoldingRegisterFunctionCode,
					readInputRegisterFunctionCode,
					readWriteMultipleRegisterFunctionCode,
					readFifoQueueFunctionCode:

					state = stateReadLength
				case writeSingleCoilFunctionCode,
					writeSingleRegisterFunctionCode,
					writeMultipleRegisterFunctionCode,
					writeMultipleCoilsFunctionCode:

					state = stateReadPayload
					toRead = 4
				case maskWriteRegisterFunctionCode:
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
			if length > rtuMaxSize-5 || length == 0 {
				return nil, &InvalidLengthError{length: length}
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

func (mb *rtuSerialTransporter) Send(ctx context.Context, aduRequest []byte) (aduResponse []byte, err error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if err = mb.connect(ctx); err != nil {
		return
	}
	mb.lastActivity = time.Now()
	mb.startCloseTimer()

	slog.Debug("send to modbus slave", "request", hex.EncodeToString(aduRequest))
	if _, err = mb.port.Write(aduRequest); err != nil {
		return
	}

	bytesToRead := calculateResponseLength(aduRequest)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(mb.calculateDelay(len(aduRequest) + bytesToRead)):
	}

	data, err := readIncrementally(aduRequest[0], aduRequest[1], mb.port, time.Now().Add(mb.Config.Timeout))
	if err != nil {
		return nil, err
	}
	slog.Debug("recv from modbus slave", "response", hex.EncodeToString(data[:]))
	aduResponse = data
	return
}

// calculateDelay is inherited from previous code...
func (mb *rtuSerialTransporter) calculateDelay(chars int) time.Duration {
	var characterDelay, frameDelay int

	if mb.BaudRate <= 0 || mb.BaudRate > 19200 {
		characterDelay = 750
		frameDelay = 1750
	} else {
		characterDelay = 15000000 / mb.BaudRate
		frameDelay = 35000000 / mb.BaudRate
	}
	return time.Duration(characterDelay*chars+frameDelay) * time.Microsecond
}

// calculateResponseLength is inherited...
func calculateResponseLength(adu []byte) int {
	length := rtuMinSize
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
