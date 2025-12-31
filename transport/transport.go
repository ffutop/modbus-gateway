// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package transport

import (
	"context"

	"github.com/ffutop/modbus-gateway/modbus"
)

// RequestHandler handles a Modbus request/response cycle.
// It takes a raw Application Data Unit (ADU) and returns a response ADU.
// The ADU format depends on the transport (RTU vs TCP).
// However, the core Dispatcher needs to be agnostic.
//
// To unify this, we might need a generic ADU interface or simply pass PDU + metadata.
// But bridging TCP <-> RTU involves stripping headers.
//
// A better approach for the Gateway:
// The Upstream (Server) receives a request. It decodes it enough to get the PDU and SlaveID.
// It calls the handler.
// The Handler (Gateway) forwards PDU + SlaveID to Downstream (Client).
// Downstream wraps it and sends.
//

type RequestHandler func(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error)

// Upstream represents a source of requests (A Modbus Master connected to us).
// It acts as a Server.
type Upstream interface {
	// Start starts the server and blocks. It should be called in a goroutine.
	// schema is the specific config (e.g. TCP address, Serial settings)
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
