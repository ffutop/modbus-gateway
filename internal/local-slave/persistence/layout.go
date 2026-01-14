package persistence

import (
	"unsafe"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
)

const (
	sizeCoils    = model.MaxAddress + 1
	sizeDiscrete = model.MaxAddress + 1
	sizeHolding  = (model.MaxAddress + 1) * 2
	sizeInput    = (model.MaxAddress + 1) * 2
	totalSize    = sizeCoils + sizeDiscrete + sizeHolding + sizeInput

	offsetCoils    = 0
	offsetDiscrete = offsetCoils + sizeCoils
	offsetHolding  = offsetDiscrete + sizeDiscrete
	offsetInput    = offsetHolding + sizeHolding
)

// mapBytesToModel constructs a DataModel backed by the provided data slice.
// Warning: This function uses unsafe pointers to cast byte slices to uint16 slices.
// The resulting DataModel relies on the host's endianness for multi-byte values.
// This provides zero-copy access but sacrifices portability across architectures
// with different endianness.
func mapBytesToModel(data []byte) *model.DataModel {
	m := &model.DataModel{}

	// Coils (Bytes)
	m.Coils = data[offsetCoils : offsetCoils+sizeCoils]

	// Discrete Inputs (Bytes)
	m.DiscreteInputs = data[offsetDiscrete : offsetDiscrete+sizeDiscrete]

	// Holding Registers (Uint16)
	holdingBytes := data[offsetHolding : offsetHolding+sizeHolding]
	m.HoldingRegisters = unsafe.Slice((*uint16)(unsafe.Pointer(&holdingBytes[0])), sizeHolding/2)

	// Input Registers (Uint16)
	inputBytes := data[offsetInput : offsetInput+sizeInput]
	m.InputRegisters = unsafe.Slice((*uint16)(unsafe.Pointer(&inputBytes[0])), sizeInput/2)

	return m
}
