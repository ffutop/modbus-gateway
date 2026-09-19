// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package local

import (
	"context"

	localslave "github.com/ffutop/modbus-gateway/internal/local-slave"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/modbus"
)

// Client implements Downstream for a business local slave backed by a
// shared simulation model.
type Client struct {
	slave *localslave.LocalSlave
}

// NewClient creates a new Local Client bound to sim. sim's lifecycle
// (persistence open/close) is owned centrally by whoever built the shared
// simulation registry, not by this Client - the same Simulation may be
// referenced by other Clients (local or injector) too.
func NewClient(sim *simulation.Simulation) *Client {
	return &Client{slave: localslave.NewLocalSlave(sim)}
}

// Send processes the PDU against the shared simulation model.
func (c *Client) Send(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	return c.slave.Process(ctx, pdu)
}

// Connect is a no-op for local slave.
func (c *Client) Connect(ctx context.Context) error {
	return nil
}

// Close is a no-op: the underlying Simulation's persistence is closed
// centrally, since it may be shared by other downstreams.
func (c *Client) Close() error {
	return nil
}
