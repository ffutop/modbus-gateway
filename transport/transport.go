// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package transport

import (
	"context"

	"github.com/ffutop/modbus-gateway/modbus"
)

// RequestHandler handles a Modbus request/response cycle.
// It takes a slaveID and PDU, forwards it to the destination, and returns the response PDU.
type RequestHandler func(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error)

// Upstream represents a source of requests (A Modbus Master connected to us).
// It acts as a Server.
type Upstream interface {
	// Start starts the server and blocks. It should be called in a goroutine.
	Start(ctx context.Context, handler RequestHandler) error
	Close() error
}

// Downstream represents a destination for requests (A Modbus Slave we connect to).
// It acts as a Client.
type Downstream interface {
	// Send sends a PDU to a specific SlaveID and returns the response PDU.
	Send(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error)
	Connect(ctx context.Context) error
	Close() error
}
