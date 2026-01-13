// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package local

import (
	"context"

	"github.com/ffutop/modbus-gateway/internal/config"
	localslave "github.com/ffutop/modbus-gateway/internal/local-slave"
	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
	"github.com/ffutop/modbus-gateway/modbus"
)

// Client implements Downstream interface for a local in-memory slave.
type Client struct {
	slave *localslave.LocalSlave
}

// NewClient creates a new Local Client.
func NewClient(cfg config.LocalConfig) *Client {
	// Initialize memory model
	m := model.NewDataModel()
	// Initialize protocol logic
	s := localslave.NewLocalSlave(m)

	return &Client{
		slave: s,
	}
}

// Send processes the PDU locally.
func (c *Client) Send(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	// The LocalSlave is synchronous and fast, so we just call Process.
	// We ignore context cancellation inside the tight memory operation logic,
	// but we could respect it if we simulated delay.
	return c.slave.Process(pdu)
}

// Connect is a no-op for local slave.
func (c *Client) Connect(ctx context.Context) error {
	return nil
}

// Close is a no-op for local slave.
func (c *Client) Close() error {
	return nil
}
