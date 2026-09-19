// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package injector adapts the "injector" downstream type: a dedicated Slave
// ID that only accepts standard Modbus writes (FC05/06/15/16) and maps them,
// per configuration, into a shared simulation's Discrete Inputs / Input
// Registers.
package injector

import (
	"context"

	"github.com/ffutop/modbus-gateway/internal/config"
	localslave "github.com/ffutop/modbus-gateway/internal/local-slave"
	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/modbus"
)

// Client implements Downstream for the injector adapter.
type Client struct {
	slave *localslave.InjectorSlave
}

// NewClient creates a new injector Client bound to sim, translating the
// config-level mappings into resolved ones. Mapping validity (table
// pairing, range bounds, non-overlap) is assumed to already have been
// checked by config.Config.Validate() at load time.
func NewClient(sim *simulation.Simulation, mappings []config.MappingConfig) *Client {
	resolved := make([]localslave.ResolvedMapping, 0, len(mappings))
	for _, m := range mappings {
		resolved = append(resolved, localslave.ResolvedMapping{
			SourceTable: tableFromName(m.Source.Table),
			SourceStart: m.Source.StartAddress,
			Count:       m.Source.Count,
			TargetTable: tableFromName(m.Target.Table),
			TargetStart: m.Target.StartAddress,
		})
	}
	return &Client{slave: localslave.NewInjectorSlave(sim, resolved)}
}

func tableFromName(name string) model.TableType {
	switch name {
	case "coils":
		return model.TableCoils
	case "discrete_inputs":
		return model.TableDiscreteInputs
	case "holding_registers":
		return model.TableHoldingRegisters
	case "input_registers":
		return model.TableInputRegisters
	default:
		// Unreachable once config.Config.Validate() has run.
		return model.TableType(-1)
	}
}

// Send processes the PDU against the shared simulation model via mapping.
func (c *Client) Send(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	return c.slave.Process(ctx, pdu)
}

// Connect is a no-op for the injector adapter.
func (c *Client) Connect(ctx context.Context) error {
	return nil
}

// Close is a no-op: the underlying Simulation's persistence is closed
// centrally, since it may be shared by other downstreams.
func (c *Client) Close() error {
	return nil
}
