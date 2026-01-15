// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.
package rtuovertcp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/ffutop/modbus-gateway/modbus"
	rtupacket "github.com/ffutop/modbus-gateway/modbus/rtu"
)

func TestServer_LifeCycle(t *testing.T) {
	// 1. Setup Server
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close() // Free port

	s := NewServer(addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Mock Handler
	handler := func(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
		if slaveID != 1 {
			t.Errorf("Handler expected slaveID 1, got %d", slaveID)
		}
		if pdu.FunctionCode == 0x03 {
			// Read Holding Registers Response
			return modbus.ProtocolDataUnit{
				FunctionCode: 0x03,
				Data:         []byte{0x02, 0xAA, 0xBB},
			}, nil
		}
		return modbus.ProtocolDataUnit{}, nil
	}

	go func() {
		if err := s.Start(ctx, handler); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()
	// Wait for server start
	time.Sleep(50 * time.Millisecond)

	// 2. Client Connection
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// 3. Send RTU Frame (Read Holding Registers)
	// Slave: 1, Func: 3, Addr: 0, Quant: 1
	reqPDU := modbus.ProtocolDataUnit{FunctionCode: 0x03, Data: []byte{0x00, 0x00, 0x00, 0x01}}
	reqADU := &rtupacket.ApplicationDataUnit{SlaveID: 1, Pdu: reqPDU}
	reqBytes, _ := reqADU.Encode()

	if _, err := conn.Write(reqBytes); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 4. Read Response
	// Using Framer to read response efficiently
	respBytes, err := rtupacket.ReadResponse(1, 0x03, conn, time.Now().Add(1*time.Second))
	if err != nil {
		t.Fatalf("ReadResponse failed: %v", err)
	}

	respADU, err := rtupacket.Decode(respBytes)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if respADU.Pdu.Data[1] != 0xAA {
		t.Errorf("Unexpected data: %X", respADU.Pdu.Data)
	}

	// 5. Cleanup
	cancel()
	s.Close()
}
