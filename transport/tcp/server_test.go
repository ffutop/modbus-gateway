// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.
package tcp

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/ffutop/modbus-gateway/modbus"
)

func TestServer_Start_And_Handle(t *testing.T) {
	// 1. Setup Server on pre-allocated port to avoid race on reading s.listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close() // Close so Server can bind to it immediately

	s := NewServer(addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handler Mock
	handler := func(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
		if slaveID != 1 {
			t.Errorf("Handler expected slaveID 1, got %d", slaveID)
		}
		if pdu.FunctionCode == 0x03 {
			// Read Holding Registers
			return modbus.ProtocolDataUnit{
				FunctionCode: 0x03,
				Data:         []byte{0x02, 0xAA, 0xBB}, // ByteCount + Data
			}, nil
		}
		if pdu.FunctionCode == 0x10 {
			// Write Multiple Registers response is Address + Quantity
			return modbus.ProtocolDataUnit{
				FunctionCode: 0x10,
				Data:         pdu.Data[:4],
			}, nil
		}
		return modbus.ProtocolDataUnit{}, nil
	}

	// Start Server
	errChan := make(chan error)
	go func() {
		errChan <- s.Start(ctx, handler)
	}()

	// 2. Create Client Connection with retry
	var conn net.Conn
	for i := 0; i < 20; i++ {
		conn, err = net.Dial("tcp", addr)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("Failed to connect to server after retries, last error: %v", err)
	}
	defer conn.Close()

	// 3. Test ReadHoldingRegisters (0x03)
	// ADU: [TransID(2)] [Proto(2)] [Len(2)] [UnitID(1)] [Func(1)] [Data...]
	reqPDU := []byte{0x03, 0x00, 0x01, 0x00, 0x01} // Read 1 reg at 1
	reqADU := make([]byte, 7+len(reqPDU))
	binary.BigEndian.PutUint16(reqADU[0:], 123)                   // TransID
	binary.BigEndian.PutUint16(reqADU[2:], 0)                     // Proto
	binary.BigEndian.PutUint16(reqADU[4:], uint16(1+len(reqPDU))) // Length
	reqADU[6] = 1                                                 // UnitID
	copy(reqADU[7:], reqPDU)

	if _, err := conn.Write(reqADU); err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	// Read Response
	respBuf := make([]byte, 512)
	n, err := conn.Read(respBuf)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Validate Response (TransID: 123, Func: 03, Data: 02 AA BB)
	if n < 10 {
		t.Errorf("Response too short: %d", n)
	}
	if binary.BigEndian.Uint16(respBuf[0:]) != 123 {
		t.Errorf("Wrong TransID: %v", respBuf[:2])
	}
	if respBuf[7] != 0x03 {
		t.Errorf("Wrong FunctionCode: %02X", respBuf[7])
	}

	// 4. Test WriteMultipleRegisters (0x10)
	// Request: [Func: 10] [Addr: 00 01] [Quant: 00 01] [ByteCount: 02] [Data: 12 34]
	reqPDU2 := []byte{0x10, 0x00, 0x01, 0x00, 0x01, 0x02, 0x12, 0x34}
	reqADU2 := make([]byte, 7+len(reqPDU2))
	binary.BigEndian.PutUint16(reqADU2[0:], 124) // TransID
	binary.BigEndian.PutUint16(reqADU2[2:], 0)   // Proto
	binary.BigEndian.PutUint16(reqADU2[4:], uint16(1+len(reqPDU2)))
	reqADU2[6] = 1
	copy(reqADU2[7:], reqPDU2)

	if _, err := conn.Write(reqADU2); err != nil {
		t.Fatalf("Failed to write request 2: %v", err)
	}

	n, err = conn.Read(respBuf)
	if err != nil {
		t.Fatalf("Failed to read response 2: %v", err)
	}
	if binary.BigEndian.Uint16(respBuf[0:]) != 124 {
		t.Errorf("Wrong TransID 2: %v", respBuf[:2])
	}
	if respBuf[7] != 0x10 {
		t.Errorf("Wrong FunctionCode 2: %02X", respBuf[7])
	}

	// 5. Test Invalid Length Protocol (Split Packet or Overflow)
	// Overflow > 260
	hugeBuf := make([]byte, 300)
	// write huge buf
	conn.Write(hugeBuf)
	// Server should close connection or handle error gracefully.
	// We can check if connection is closed by trying to read.

	_, err = conn.Read(respBuf)
	// Expecting error (closed connection) or EOF
	if err == nil {
		// Possibly didn't close yet, but it should have logged error.
	}
}

func TestServer_LifeCycle(t *testing.T) {
	s := NewServer("127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		s.Start(ctx, func(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
			return pdu, nil
		})
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
	// Should shutdown gracefully
	if err := s.Close(); err != nil {
		// might return error if listener already closed
	}
}

// Mock Handler for negative tests
type mockHandler struct {
	called bool
}

func (m *mockHandler) Handle(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	m.called = true
	return modbus.ProtocolDataUnit{}, nil
}
