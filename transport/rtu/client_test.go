// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.
package rtu

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/modbus/crc"
)

func TestClient_Send(t *testing.T) {
	// 1. Setup Mock interactions
	// Request: 03 (Read Holding) 00 00 00 01
	// Response: 03 02 AA BB
	reqPDU := []byte{0x00, 0x00, 0x00, 0x01}
	respData := []byte{0x02, 0xAA, 0xBB}

	// Calculate Expected ADU Request (to verify what Client sends)
	// SlaveID(1) + Func(3) + Data + CRC
	expectedReq := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01}
	var c crc.CRC
	c.Reset().PushBytes(expectedReq)
	sum := c.Value()
	expectedReq = append(expectedReq, byte(sum), byte(sum>>8))

	// Prepare Response Buffer (what Client reads)
	// SlaveID(1) + Func(3) + Data + CRC
	respADU := []byte{0x01, 0x03}
	respADU = append(respADU, respData...)
	c.Reset().PushBytes(respADU)
	sumResp := c.Value()
	respADU = append(respADU, byte(sumResp), byte(sumResp>>8))

	// 2. Setup Client with Mock Port
	// We need to inject the mock port. verify NewClient logic.
	// NewClient creates a `serialPort`. `serialPort` opens real port on `Connect`.
	// verification shows `client.serialPort.port` is an `io.ReadWriteCloser`.
	// If we set it manually, we might bypass Open?
	// `client.Send` calls `mb.connect(ctx)`.
	// `connect` checks `if modbus.port == nil`.
	// So if we pre-set `modbus.port`, `connect` should skip `serial.Open`.

	writer := &bytes.Buffer{}
	reader := bytes.NewReader(respADU)
	mock := &mockPort{Reader: reader, Writer: writer}

	client := NewClient(config.SerialConfig{})
	// Inject Mock
	client.rtuSerialTransporter.port = mock
	// We also need to set timeout to avoid panic or nil deref if used
	client.Config.Timeout = 100 * time.Millisecond

	// 3. Execute Send
	ctx := context.Background()
	pdu := modbus.ProtocolDataUnit{FunctionCode: 0x03, Data: reqPDU}

	resp, err := client.Send(ctx, 1, pdu)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// 4. Verify Request
	sentBytes := writer.Bytes()
	if !bytes.Equal(sentBytes, expectedReq) {
		t.Errorf("Request mismatch.\nWant: %X\nGot:  %X", expectedReq, sentBytes)
	}

	// 5. Verify Response PDU
	if resp.FunctionCode != 0x03 {
		t.Errorf("Response Func mismatch: %02X", resp.FunctionCode)
	}
	if !bytes.Equal(resp.Data, respData) {
		t.Errorf("Response Data mismatch.\nWant: %X\nGot:  %X", respData, resp.Data)
	}
}

func TestClient_CRCError(t *testing.T) {
	// Construct Response with BAD CRC
	respADU := []byte{0x01, 0x03, 0x02, 0xAA, 0xBB, 0xFF, 0xFF} // Bad CRC

	writer := &bytes.Buffer{}
	reader := bytes.NewReader(respADU)
	mock := &mockPort{Reader: reader, Writer: writer}

	client := NewClient(config.SerialConfig{})
	client.rtuSerialTransporter.port = mock
	client.Config.Timeout = 100 * time.Millisecond

	ctx := context.Background()
	pdu := modbus.ProtocolDataUnit{FunctionCode: 0x03, Data: []byte{0x00, 0x00, 0x00, 0x01}}

	_, err := client.Send(ctx, 1, pdu)
	if err == nil {
		t.Error("Expected CRC error, got nil")
	} else {
		// t.Log("Got expected error:", err)
	}
}
