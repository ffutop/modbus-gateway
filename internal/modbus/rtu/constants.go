// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtu

const (
	MinSize = 4
	MaxSize = 256

	ExceptionSize = 5
)

// Function Codes
const (
	FuncCodeReadCoils           = 0x01
	FuncCodeReadDiscreteInputs  = 0x02
	FuncCodeReadHoldingRegister = 0x03
	FuncCodeReadInputRegister   = 0x04

	FuncCodeWriteSingleCoil       = 0x05
	FuncCodeWriteSingleRegister   = 0x06
	FuncCodeWriteMultipleCoils    = 0x0F
	FuncCodeWriteMultipleRegister = 0x10
	FuncCodeMaskWriteRegister     = 0x16

	FuncCodeReadWriteMultipleRegister = 0x17
	FuncCodeReadFIFOQueue             = 0x18
)
