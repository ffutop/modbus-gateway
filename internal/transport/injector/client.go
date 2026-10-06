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
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/internal/simulation/model"
)

// Client implements Downstream for the injector adapter. It only accepts
// FC05/FC06/FC15/FC16 and translates a write landing fully inside one
// configured mapping into a write against the shared simulation's target
// table. It never modifies Coils/HoldingRegisters, and never serves reads.
type Client struct {
	sim      *simulation.Simulation
	mappings []mapping
}

// NewClient creates a new injector Client bound to sim, translating the
// config-level mappings into resolved ones. Mapping validity (table
// pairing, range bounds, non-overlap) is assumed to already have been
// checked by config.Config.Validate() at load time.
func NewClient(sim *simulation.Simulation, mappings []config.MappingConfig) *Client {
	resolved := make([]mapping, 0, len(mappings))
	for _, m := range mappings {
		resolved = append(resolved, mapping{
			SourceTable: tableFromName(m.Source.Table),
			SourceStart: m.Source.StartAddress,
			Count:       m.Source.Count,
			TargetTable: tableFromName(m.Target.Table),
			TargetStart: m.Target.StartAddress,
		})
	}
	return newClient(sim, resolved)
}

func newClient(sim *simulation.Simulation, mappings []mapping) *Client {
	return &Client{sim: sim, mappings: mappings}
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

// Connect is a no-op for the injector adapter.
func (c *Client) Connect(ctx context.Context) error {
	return nil
}

// Close is a no-op: the underlying Simulation's persistence is closed
// centrally, since it may be shared by other downstreams.
func (c *Client) Close() error {
	return nil
}
