// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package api

import "github.com/ffutop/modbus-gateway/internal/config"

// runningTree describes the immutable normalized startup configuration, also
// exposing the synthesized simulations of legacy configurations.
func runningTree(c *config.Config) map[string]any {
	gateways := []any{}
	sims := []any{}
	if c != nil {
		for _, s := range c.Simulations {
			sims = append(sims, map[string]any{"name": s.Name, "persistence": map[string]any{"type": s.Persistence.Type, "path": s.Persistence.Path}})
		}
		for _, g := range c.Gateways {
			up, down := []any{}, []any{}
			for _, u := range g.Upstreams {
				up = append(up, map[string]any{"type": u.Type, "tcp": map[string]any{"address": u.Tcp.Address}, "serial": serialTree(u.Serial)})
			}
			for i, d := range g.Downstreams {
				m := map[string]any{"name": d.DisplayName(i), "type": d.Type, "slave_ids": d.SlaveIDs, "tcp": map[string]any{"address": d.Tcp.Address}, "serial": serialTree(d.Serial)}
				if d.SimulationRef != "" {
					m["simulation"] = map[string]any{"ref": d.SimulationRef}
				}
				down = append(down, m)
			}
			gateways = append(gateways, map[string]any{"name": g.Name, "upstreams": up, "downstreams": down})
		}
	}
	return map[string]any{"gateways": gateways, "simulations": sims}
}

func serialTree(s config.SerialConfig) map[string]any {
	return map[string]any{"device": s.Device, "baud_rate": s.BaudRate, "data_bits": s.DataBits, "parity": s.Parity, "stop_bits": s.StopBits, "timeout": s.Timeout.String()}
}
