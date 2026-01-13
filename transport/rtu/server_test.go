// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.
package rtu

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/modbus/crc"
)

func TestCalculateRequestLength(t *testing.T) {
	tests := []struct {
		name     string
		funcCode byte
		header   []byte
		want     int
		wantErr  bool
	}{
		{"ReadHoldingRegisters", 0x03, []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01}, 8, false},
		{"WriteSingleRegister", 0x06, []byte{0x01, 0x06, 0x00, 0x00, 0xAA, 0xBB}, 8, false},
		{"WriteMultipleRegisters_ShortHeader", 0x10, []byte{0x01, 0x10, 0x00, 0x01, 0x00, 0x01}, 0, true},
		{"WriteMultipleRegisters_Valid", 0x10, []byte{0x01, 0x10, 0x00, 0x01, 0x00, 0x01, 0x02}, 7 + 2 + 2, false},
		{"UnknownFunction", 0x99, []byte{0x01, 0x99}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateRequestLength(tt.funcCode, tt.header)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateRequestLength() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("calculateRequestLength() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockPort struct {
	io.Reader
	io.Writer
}

func (m *mockPort) Close() error { return nil }

func TestScanLoop(t *testing.T) {
	// Construct a valid ReadHoldingRegisters request (0x03)
	// Slave: 01, Func: 03, Addr: 0000, Quant: 0001
	// PDU: 03 00 00 00 01
	// ADU: 01 03 00 00 00 01 + CRC
	reqPDU := []byte{0x03, 0x00, 0x00, 0x00, 0x01}
	reqADU := []byte{0x01}
	reqADU = append(reqADU, reqPDU...)

	var c crc.CRC
	c.Reset().PushBytes(reqADU)
	sum := c.Value()
	reqADU = append(reqADU, byte(sum>>8), byte(sum)) // Modbus is Big Endian?
	// Wait. adu.go Encode:
	// raw[length-1] = byte(checksum >> 8) -> High
	// raw[length-2] = byte(checksum)      -> Low
	// Modbus spec says CRC is Low Byte first, then High Byte.
	// But let's check what adu.go (my codebase) expects.
	// Decode: checksum := uint16(raw[length-1])<<8 | uint16(raw[length-2])
	// Value at len-1 is shifted left. So len-1 is High Byte.
	// Value at len-2 is Low Byte.
	// So layout in memory: [..., Low, High].
	// My construction: append(..., byte(sum), byte(sum>>8)) == [Low, High].
	// This Matches adu.go.
	// Wait, I appended High, Low in this string literal? No.
	// Let's be explicit.

	// Re-construct for clarity
	reqADU = []byte{0x01}
	reqADU = append(reqADU, reqPDU...)

	c.Reset().PushBytes(reqADU)
	sum = c.Value()

	// Appending Low, High
	reqADU = append(reqADU, byte(sum))
	reqADU = append(reqADU, byte(sum>>8))

	// Clean Input
	input := reqADU

	reader := bytes.NewReader(input)
	writer := &bytes.Buffer{}

	port := &mockPort{Reader: reader, Writer: writer}

	s := &Server{}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	received := make(chan bool)

	handler := func(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
		if slaveID != 0x01 {
			t.Errorf("Handler got slaveID %v, want 1", slaveID)
		}
		if pdu.FunctionCode != 0x03 {
			t.Errorf("Handler got func %v, want 3", pdu.FunctionCode)
		}
		close(received)
		// Return dummy response
		return modbus.ProtocolDataUnit{FunctionCode: 0x03, Data: []byte{0x02, 0x00, 0x00}}, nil
	}

	go s.scanLoop(ctx, port, handler)

	// Test Wait
	select {
	case <-received:
		// Success
	case <-time.After(300 * time.Millisecond):
		t.Error("Handler not called")
	}

	if writer.Len() == 0 {
		t.Error("Simulated response not written")
	}
}

func TestServer_FunctionCodes(t *testing.T) {
	// Table driven test for various function codes to ensure loop handles them
	tests := []struct {
		name     string
		funcCode byte
		reqPDU   []byte // Just the PDU part (Func + Data)
		wantLen  int    // expected total ADU length (Slave + PDU + CRC)
	}{
		{"ReadCoils", 0x01, []byte{0x01, 0x00, 0x00, 0x00, 0x01}, 8},
		{"WriteSingleRegister", 0x06, []byte{0x06, 0x00, 0x00, 0xAA, 0xBB}, 8},
		// 0x10 Header: Func(1)+Addr(2)+Quant(2)+ByteCount(1) + Data(N)
		// 0x10 Write 2 Regs (4 bytes)
		{"WriteMultipleRegisters", 0x10, []byte{0x10, 0x00, 0x01, 0x00, 0x02, 0x04, 0x11, 0x22, 0x33, 0x44}, 1 + 1 + 2 + 2 + 1 + 4 + 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Construct ADU
			reqADU := []byte{0x01} // SlaveID
			reqADU = append(reqADU, tt.reqPDU...)

			// Append CRC
			var c crc.CRC
			c.Reset().PushBytes(reqADU)
			sum := c.Value()
			reqADU = append(reqADU, byte(sum), byte(sum>>8))

			// Write to mock
			reader := bytes.NewReader(reqADU)
			writer := &bytes.Buffer{}
			port := &mockPort{Reader: reader, Writer: writer}

			s := &Server{}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			handled := make(chan bool)
			handler := func(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
				if pdu.FunctionCode != tt.funcCode {
					t.Errorf("Want func %d, got %d", tt.funcCode, pdu.FunctionCode)
				}
				close(handled)
				return modbus.ProtocolDataUnit{FunctionCode: tt.funcCode, Data: []byte{}}, nil
			}

			go s.scanLoop(ctx, port, handler)

			select {
			case <-handled:
			case <-time.After(150 * time.Millisecond):
				t.Error("Handler not called for", tt.name)
			}
		})
	}
}
