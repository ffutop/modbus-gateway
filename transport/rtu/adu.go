// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtu

import (
	"fmt"

	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/modbus/crc"
)

const (
	rtuMinSize = 4
	rtuMaxSize = 256

	rtuExceptionSize = 5
)

// rtuPackager implements Packager interface.
type ApplicationDataUnit struct {
	SlaveID byte
	Pdu     modbus.ProtocolDataUnit
	crc     crc.CRC
}

func Decode(raw []byte) (adu *ApplicationDataUnit, err error) {
	length := len(raw)
	// Minimum size (including address, function and CRC)
	if length < rtuMinSize {
		err = fmt.Errorf("modbus: request length '%v' does not meet minimum '%v'", length, rtuMinSize)
		return
	}

	// Calculate checksum
	var crc crc.CRC
	crc.Reset().PushBytes(raw[0 : length-2])
	checksum := uint16(raw[length-1])<<8 | uint16(raw[length-2])
	if checksum != crc.Value() {
		err = fmt.Errorf("modbus: response crc '%v' does not match expected '%v'", checksum, crc.Value())
		return
	}
	adu = &ApplicationDataUnit{}
	adu.SlaveID = raw[0]
	adu.Pdu.FunctionCode = raw[1]
	adu.Pdu.Data = raw[2 : length-2]
	adu.crc = crc
	return
}

// Encode encodes PDU in an RTU frame:
//
//	Slave Address   : 1 byte
//	Function        : 1 byte
//	Data            : 0 up to 252 bytes
//	CRC             : 2 bytes
func (adu *ApplicationDataUnit) Encode() (raw []byte, err error) {
	length := len(adu.Pdu.Data) + 4
	if length > rtuMaxSize {
		err = fmt.Errorf("modbus: length of data '%v' must not be bigger than '%v'", length, rtuMaxSize)
		return
	}
	raw = make([]byte, length)

	raw[0] = adu.SlaveID
	raw[1] = adu.Pdu.FunctionCode
	copy(raw[2:], adu.Pdu.Data)

	// Append crc
	var crc crc.CRC
	crc.Reset().PushBytes(raw[0 : length-2])
	checksum := crc.Value()

	raw[length-1] = byte(checksum >> 8)
	raw[length-2] = byte(checksum)
	return
}

// Verify verifies response length and slave id.
func (req *ApplicationDataUnit) Verify(resp *ApplicationDataUnit) (err error) {
	length := len(resp.Pdu.Data) + 4
	// Minimum size (including address, function and CRC)
	if length < rtuMinSize {
		err = fmt.Errorf("modbus: response length '%v' does not meet minimum '%v'", length, rtuMinSize)
		return
	}
	// Slave address must match
	if req.SlaveID != resp.SlaveID {
		err = fmt.Errorf("modbus: response slave id '%v' does not match request '%v'", resp.SlaveID, req.SlaveID)
		return
	}
	return
}
