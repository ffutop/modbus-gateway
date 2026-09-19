// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package localslave

import (
	"context"
	"encoding/binary"
	"log/slog"

	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/transport"
)

// exception builds a Modbus exception response PDU. Shared by LocalSlave and
// InjectorSlave.
func exception(funcCode byte, code byte) modbus.ProtocolDataUnit {
	return modbus.ProtocolDataUnit{
		FunctionCode: funcCode | 0x80,
		Data:         []byte{code},
	}
}

// packReadResponse builds a standard "byte count + data" read response PDU.
func packReadResponse(funcCode byte, data []byte) modbus.ProtocolDataUnit {
	respData := make([]byte, 1+len(data))
	respData[0] = byte(len(data))
	copy(respData[1:], data)
	return modbus.ProtocolDataUnit{FunctionCode: funcCode, Data: respData}
}

// packWriteResponse builds a standard "address + quantity" write response PDU
// (used by FC15/FC16).
func packWriteResponse(funcCode byte, address, quantity uint16) modbus.ProtocolDataUnit {
	respData := make([]byte, 4)
	binary.BigEndian.PutUint16(respData[0:2], address)
	binary.BigEndian.PutUint16(respData[2:4], quantity)
	return modbus.ProtocolDataUnit{FunctionCode: funcCode, Data: respData}
}

// auditWrite records a structured audit log entry for a write against a
// shared simulation model: simulation name, entry kind (local/injector),
// source connection, target range, commit version and result.
func auditWrite(ctx context.Context, sim *simulation.Simulation, entry, table string, address, quantity uint16, err error) {
	result := "ok"
	if err != nil {
		result = "rejected: " + err.Error()
	}
	slog.Info("simulation write",
		"simulation", sim.Name,
		"entry", entry,
		"source", transport.SourceAddr(ctx),
		"table", table,
		"address", address,
		"quantity", quantity,
		"version", sim.Version(),
		"status", sim.Status(),
		"result", result,
	)
}
