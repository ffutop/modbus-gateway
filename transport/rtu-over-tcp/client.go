// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtuovertcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ffutop/modbus-gateway/modbus"
	rtupacket "github.com/ffutop/modbus-gateway/modbus/rtu"
)

const (
	tcpTimeout = 10 * time.Second
)

// Client implements Downstream interface (Modbus RTU over TCP Client).
type Client struct {
	Address string
	Timeout time.Duration

	mu   sync.Mutex
	conn net.Conn
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

	// Ensure connection is open
	if err := mb.connect(); err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("modbus: failed to connect to %s: %w", mb.Address, err)
	}

	adu := &rtupacket.ApplicationDataUnit{
		SlaveID: slaveID,
		Pdu:     pdu,
	}

	aduBytes, err := adu.Encode()
	if err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to encode ADU: %w", err)
	}

	// Set Deadline for the interaction
	if err = mb.conn.SetDeadline(time.Now().Add(mb.Timeout)); err != nil {
		mb.close()
		return modbus.ProtocolDataUnit{}, err
	}

	// Send Request
	if _, err := mb.conn.Write(aduBytes); err != nil {
		mb.close() // Close connection on write failure to force reconnect next time
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to write to connection: %w", err)
	}

	// Read Response
	// We use the same RTU framing logic because RTU-over-TCP is just RTU frames sent over TCP.
	respBytes, err := rtupacket.ReadResponse(slaveID, pdu.FunctionCode, mb.conn, time.Now().Add(mb.Timeout))
	if err != nil {
		mb.close() // Close connection on read failure
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to read response: %w", err)
	}

	// Decode Response
	respAdu, err := rtupacket.Decode(respBytes)
	if err != nil {
		// Framing/CRC error might not imply broken connection, but for safety in RTU-over-TCP (stream desync),
		// it is often better to reset.
		// However, purely bad data shouldn't necessarily kill the TCP link.
		// We'll keep the connection unless it's a critical IO error, but ReadResponse would have caught IO errors.
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to decode response ADU: %w", err)
	}

	// Verify
	if err := adu.Verify(respAdu); err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("verification failed: %w", err)
	}

	return respAdu.Pdu, nil
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
