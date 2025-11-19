// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package tcp

import (
	"fmt"

	"github.com/ffutop/modbus-gateway/modbus"
)

const (
	tcpMinSize = 8
	tcpMaxSize = 260
)

type ApplicationDataUnit struct {
	TransactionID uint16
	ProtocolID    uint16
	Length        uint16
	SlaveID       byte
	Pdu           modbus.ProtocolDataUnit
}

func Decode(raw []byte) (adu *ApplicationDataUnit, err error) {
	if len(raw) < tcpMinSize {
		err = fmt.Errorf("modbus: request length '%v' does not meet minimum '%v'", len(raw), tcpMinSize)
		return
	}
	adu = &ApplicationDataUnit{}
	adu.TransactionID = uint16(raw[0])<<8 | uint16(raw[1])
	adu.ProtocolID = uint16(raw[2])<<8 | uint16(raw[3])
	adu.Length = uint16(raw[4])<<8 | uint16(raw[5])
	adu.SlaveID = raw[6]
	adu.Pdu.FunctionCode = raw[7]
	adu.Pdu.Data = raw[8:]
	return
}

func (adu *ApplicationDataUnit) Encode() (raw []byte, err error) {
	length := len(adu.Pdu.Data) + 8
	if length > tcpMaxSize {
		err = fmt.Errorf("modbus: length of data '%v' must not be bigger than '%v'", length, tcpMaxSize)
		return
	}
	raw = make([]byte, length)

	raw[0] = byte(adu.TransactionID >> 8)
	raw[1] = byte(adu.TransactionID >> 0)
	raw[2] = byte(adu.ProtocolID >> 8)
	raw[3] = byte(adu.ProtocolID >> 0)
	raw[4] = byte(adu.Length >> 8)
	raw[5] = byte(adu.Length >> 0)
	raw[6] = adu.SlaveID
	raw[7] = adu.Pdu.FunctionCode
	copy(raw[8:], adu.Pdu.Data)

	return
}

func (req *ApplicationDataUnit) Verify(resp *ApplicationDataUnit) (err error) {
	// Transaction ID must match
	if resp.TransactionID != req.TransactionID {
		err = fmt.Errorf("modbus: response transaction id '%v' does not match request '%v'", resp.TransactionID, req.TransactionID)
		return
	}
	return
}
