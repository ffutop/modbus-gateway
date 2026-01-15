// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtuovertcp

import (
	"context"
	"fmt"
	"net"
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
	adu := &rtupacket.ApplicationDataUnit{
		SlaveID: slaveID,
		Pdu:     pdu,
	}

	aduBytes, err := adu.Encode()
	if err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to encode ADU: %w", err)
	}

	// Dialing a new connection for simplicity
	// In production, you might want a persistent connection pool
	conn, err := net.DialTimeout("tcp", mb.Address, mb.Timeout)
	if err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("modbus: failed to connect to %s: %w", mb.Address, err)
	}
	defer conn.Close()

	if err = conn.SetDeadline(time.Now().Add(mb.Timeout)); err != nil {
		return modbus.ProtocolDataUnit{}, err
	}

	// Send Request
	if _, err := conn.Write(aduBytes); err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to write to connection: %w", err)
	}

	// Read Response
	// We use the same RTU framing logic because RTU-over-TCP is just RTU frames sent over TCP.
	respBytes, err := rtupacket.ReadResponse(slaveID, pdu.FunctionCode, conn, time.Now().Add(mb.Timeout))
	if err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to read response: %w", err)
	}

	// Decode Response
	respAdu, err := rtupacket.Decode(respBytes)
	if err != nil {
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
	// Check if address is valid
	_, err := net.ResolveTCPAddr("tcp", mb.Address)
	return err
}

// Close implements Connector interface.
func (mb *Client) Close() error {
	return nil
}