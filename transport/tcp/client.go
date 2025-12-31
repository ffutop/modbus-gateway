// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package tcp

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/ffutop/modbus-gateway/modbus"
)

const (
	tcpTimeout = 10 * time.Second
)

// Client implements Downstream interface (Modbus TCP Client).
type Client struct {
	Address string
	Timeout time.Duration

	transactionID uint32 // Atomic counter
}

// NewClient allocates and initializes a TCP Client.
func NewClient(address string) *Client {
	return &Client{
		Address: address,
		Timeout: tcpTimeout,
	}
}

// Send sends a PDU to a Slave (Downstream) and returns the response PDU.
func (mb *Client) Send(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	// 1. Construct ADU
	// Transaction ID: Incrementing
	tid := uint16(atomic.AddUint32(&mb.transactionID, 1))

	adu := &ApplicationDataUnit{
		TransactionID: tid,
		ProtocolID:    0,
		Length:        uint16(1 + len(pdu.Data)), // SlaveID + Data
		SlaveID:       slaveID,                   // Unit Identifier
		Pdu:           pdu,
	}

	aduBytes, err := adu.Encode()
	if err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to encode ADU: %w", err)
	}

	// 2. Send (Dialing a new connection for simplicity as per original design, could verify persistence)
	// For better performance, we should keep connection alive.
	// But let's stick to existing logic for now, or improve it slightly?
	// Existing logic was DialTimeout per request.

	conn, err := net.DialTimeout("tcp", mb.Address, mb.Timeout)
	if err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("modbus: failed to connect to %s: %w", mb.Address, err)
	}
	defer conn.Close()

	if err = conn.SetDeadline(time.Now().Add(mb.Timeout)); err != nil {
		return modbus.ProtocolDataUnit{}, err
	}

	respBytes, err := mb.sendAndRead(conn, aduBytes)
	if err != nil {
		return modbus.ProtocolDataUnit{}, err
	}

	// 3. Decode Response
	respAdu, err := Decode(respBytes)
	if err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to decode response ADU: %w", err)
	}

	// 4. Verify
	if err := adu.Verify(respAdu); err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("verification failed: %w", err)
	}

	return respAdu.Pdu, nil
}

func (mb *Client) sendAndRead(conn net.Conn, aduRequest []byte) ([]byte, error) {
	if _, err := conn.Write(aduRequest); err != nil {
		return nil, err
	}

	// Read MBAP Header (first 6 bytes)
	mbapHeader := make([]byte, 6)
	if _, err := io.ReadFull(conn, mbapHeader); err != nil {
		return nil, err
	}

	// Parse Length
	length := int(mbapHeader[4])<<8 | int(mbapHeader[5])

	// Read remaining bytes (UnitID + PDU)
	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}

	// Combine header and payload
	response := make([]byte, 6+length)
	copy(response, mbapHeader)
	copy(response[6:], payload)

	slog.Debug("recv from modbus tcp slave", "response", hex.EncodeToString(response))
	return response, nil
}

// Connect implements Connector interface.
func (mb *Client) Connect(ctx context.Context) error {
	// Check if address is valid
	_, err := net.ResolveTCPAddr("tcp", mb.Address)
	return err
}

// Close implements Connector interface.
func (mb *Client) Close() error {
	return nil
}
