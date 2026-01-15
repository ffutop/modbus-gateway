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
	"sync"
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

	mu            sync.Mutex
	conn          net.Conn
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
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if err := mb.connect(); err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("modbus: failed to connect to %s: %w", mb.Address, err)
	}

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

	if err := mb.conn.SetDeadline(time.Now().Add(mb.Timeout)); err != nil {
		mb.close()
		return modbus.ProtocolDataUnit{}, err
	}

	respBytes, err := mb.sendAndRead(mb.conn, aduBytes)
	if err != nil {
		mb.close() // Disconnect on IO error
		return modbus.ProtocolDataUnit{}, err
	}

	// Decode Response
	respAdu, err := Decode(respBytes)
	if err != nil {
		// Try to keep connection open on decode error, unless it's critical
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to decode response ADU: %w", err)
	}

	// Verify
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
	mb.mu.Lock()
	defer mb.mu.Unlock()
	return mb.connect()
}

// Close implements Connector interface.
func (mb *Client) Close() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.close()
	return nil
}

// connect ensures there is an active connection. Caller must hold the mutex.
func (mb *Client) connect() error {
	if mb.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", mb.Address, mb.Timeout)
	if err != nil {
		return err
	}
	mb.conn = conn
	return nil
}

// close closes the connection and resets the state. Caller must hold the mutex.
func (mb *Client) close() {
	if mb.conn != nil {
		mb.conn.Close()
		mb.conn = nil
	}
}