// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package modbus

import "encoding/binary"

// NewException builds the exception response to a request with funcCode.
func NewException(funcCode, code byte) ProtocolDataUnit {
	return ProtocolDataUnit{FunctionCode: funcCode | 0x80, Data: []byte{code}}
}

// NewReadResponse builds a "byte count + data" read response.
func NewReadResponse(funcCode byte, data []byte) ProtocolDataUnit {
	respData := make([]byte, 1+len(data))
	respData[0] = byte(len(data))
	copy(respData[1:], data)
	return ProtocolDataUnit{FunctionCode: funcCode, Data: respData}
}

// NewWriteResponse builds an "address + quantity" write response (FC15/FC16).
func NewWriteResponse(funcCode byte, address, quantity uint16) ProtocolDataUnit {
	respData := make([]byte, 4)
	binary.BigEndian.PutUint16(respData[0:2], address)
	binary.BigEndian.PutUint16(respData[2:4], quantity)
	return ProtocolDataUnit{FunctionCode: funcCode, Data: respData}
}
