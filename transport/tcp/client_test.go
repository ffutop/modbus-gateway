// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.
package tcp

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/ffutop/modbus-gateway/modbus"
)

func TestClient_Send(t *testing.T) {
	// 1. Setup Mock Server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 512)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					// Parse ADU to verify
					// TransactionID (0-1), ProtocolID (2-3), Length (4-5), UnitID (6), Func (7)
					if n < 8 {
						continue
					}
					transID := binary.BigEndian.Uint16(buf[0:])
					// unitID := buf[6]
					funcCode := buf[7]

					// Construct Response
					// Echo TransactionID, matching UnitID/Func
					// ReadHoldingRegisters (0x03) -> Return 2 bytes: AA BB
					respPDU := []byte{funcCode, 0x02, 0xAA, 0xBB}
					respADU := make([]byte, 7+len(respPDU))
					binary.BigEndian.PutUint16(respADU[0:], transID)
					binary.BigEndian.PutUint16(respADU[2:], 0)
					binary.BigEndian.PutUint16(respADU[4:], uint16(1+len(respPDU)))
					respADU[6] = buf[6] // UnitID
					copy(respADU[7:], respPDU)

					c.Write(respADU)
				}
			}(conn)
		}
	}()

	// 2. Setup Client
	client := NewClient(listener.Addr().String())
	client.Timeout = 1 * time.Second
	defer client.Close()

	// 3. Test Send
	pdu := modbus.ProtocolDataUnit{
		FunctionCode: 0x03,
		Data:         []byte{0x00, 0x01, 0x00, 0x01},
	}
	ctx := context.Background()
	resp, err := client.Send(ctx, 1, pdu)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if resp.FunctionCode != 0x03 {
		t.Errorf("Expected funcCode 0x03, got %02X", resp.FunctionCode)
	}
	if len(resp.Data) != 3 { // ByteCount + 2 Bytes
		t.Errorf("Expected 3 data bytes, got %d", len(resp.Data))
	} else {
		if resp.Data[1] != 0xAA {
			t.Errorf("Data mismatch")
		}
	}
}

func TestClient_Timeout(t *testing.T) {
	// 1. Setup Hanging Server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			// Read but never write back
			buf := make([]byte, 10)
			conn.Read(buf)
			time.Sleep(2 * time.Second) // Wait longer than client timeout
			conn.Close()
		}
	}()

	client := NewClient(listener.Addr().String())
	client.Timeout = 200 * time.Millisecond // Short timeout
	defer client.Close()

	pdu := modbus.ProtocolDataUnit{
		FunctionCode: 0x01,
		Data:         []byte{0x00, 0x00, 0x00, 0x01},
	}
	_, err = client.Send(context.Background(), 1, pdu)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

func TestClient_MalformedResponse(t *testing.T) {
	// 1. Send garbage
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			buf := make([]byte, 512)
			conn.Read(buf)
			// Write garbage
			conn.Write([]byte{0x00, 0x01, 0x00}) // Too short header
			conn.Close()
		}
	}()

	client := NewClient(listener.Addr().String())
	client.Timeout = 1 * time.Second
	defer client.Close()

	pdu := modbus.ProtocolDataUnit{FunctionCode: 0x01, Data: []byte{0x00}}
	_, err = client.Send(context.Background(), 1, pdu)
	if err == nil {
		t.Error("Expected error on malformed response")
	} else if err == io.EOF {
		// Acceptable if it detected disconnect
	} else {
		// Acceptable
	}
}
