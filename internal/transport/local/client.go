// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package local adapts the "local" downstream type: a Slave ID served
// in-process from a shared simulation model, implementing the standard
// Modbus functions FC01-06, FC15 and FC16.
package local

import (
	"context"

	"github.com/ffutop/modbus-gateway/internal/simulation"
)

// Client implements Downstream for a local slave backed by a shared
// simulation model. Several clients (local or injector) may share one
// Simulation.
type Client struct {
	sim *simulation.Simulation
}

// NewClient creates a Client bound to sim. sim's lifecycle (persistence
// open/close) is owned by whoever built the shared simulation registry, not
// by this Client.
func NewClient(sim *simulation.Simulation) *Client {
	return &Client{sim: sim}
}

// Connect is a no-op for a local slave.
func (c *Client) Connect(ctx context.Context) error {
	return nil
}

// Close is a no-op: the underlying Simulation's persistence is closed
// centrally, since it may be shared by other downstreams.
func (c *Client) Close() error {
	return nil
}
