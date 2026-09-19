// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package config

import (
	"fmt"

	"github.com/ffutop/modbus-gateway/internal/routing"
)

// addrRange is a half-open [start, end) range over the 0..65535 address space.
type addrRange struct {
	start, end uint32
}

func (r addrRange) overlaps(o addrRange) bool {
	return r.start < o.end && o.start < r.end
}

// Validate checks cross-cutting rules that can only be enforced once the
// whole config has been parsed and normalized: simulation references,
// mapping validity/overlap (both within an injector and across every
// injector sharing a simulation), single-slave-ID entries, and (v1 only)
// duplicate upstream listen addresses. It runs for both v0 and v1 configs;
// v0 configs simply never exercise the injector/mapping branches.
func (c *Config) Validate() error {
	simNames := make(map[string]bool, len(c.Simulations))
	for _, s := range c.Simulations {
		if s.Name == "" {
			return fmt.Errorf("config: simulation name must not be empty")
		}
		if simNames[s.Name] {
			return fmt.Errorf("config: duplicate simulation name %q", s.Name)
		}
		simNames[s.Name] = true
	}

	// Target ranges, keyed by "<simulation>|<table>", accumulated across
	// every injector downstream (possibly in different gateways) that
	// references the same simulation.
	targetsBySimTable := make(map[string][]addrRange)

	// Upstream listen addresses/devices, v1 only.
	listenSeen := make(map[string]string)

	for _, gw := range c.Gateways {
		if c.Version == 1 {
			for _, us := range gw.Upstreams {
				if err := checkListenConflict(listenSeen, gw.Name, us); err != nil {
					return err
				}
			}
		}

		for _, ds := range gw.Downstreams {
			switch ds.Type {
			case "local":
				if err := validateSimRef(simNames, ds.SimulationRef); err != nil {
					return fmt.Errorf("gateway %q downstream %q: %w", gw.Name, ds.Name, err)
				}
				if c.Version == 1 {
					if len(ds.Mappings) > 0 {
						return fmt.Errorf("gateway %q downstream %q: 'local' must not declare simulation.mappings", gw.Name, ds.Name)
					}
					if err := validateSingleSlaveID(ds.SlaveIDs); err != nil {
						return fmt.Errorf("gateway %q downstream %q: %w", gw.Name, ds.Name, err)
					}
				}

			case "injector":
				if c.Version != 1 {
					return fmt.Errorf("gateway %q downstream %q: downstream type 'injector' requires version: 1", gw.Name, ds.Name)
				}
				if err := validateSimRef(simNames, ds.SimulationRef); err != nil {
					return fmt.Errorf("gateway %q downstream %q: %w", gw.Name, ds.Name, err)
				}
				if err := validateSingleSlaveID(ds.SlaveIDs); err != nil {
					return fmt.Errorf("gateway %q downstream %q: %w", gw.Name, ds.Name, err)
				}
				if len(ds.Mappings) == 0 {
					return fmt.Errorf("gateway %q downstream %q: 'injector' requires at least one mapping", gw.Name, ds.Name)
				}
				if err := validateMappings(ds, targetsBySimTable); err != nil {
					return fmt.Errorf("gateway %q downstream %q: %w", gw.Name, ds.Name, err)
				}

			default:
				// tcp/rtu/rtu-over-tcp: no simulation-specific rules.
			}
		}
	}

	return nil
}

func validateSimRef(names map[string]bool, ref string) error {
	if ref == "" {
		return fmt.Errorf("missing simulation.ref")
	}
	if !names[ref] {
		return fmt.Errorf("unknown simulation ref %q", ref)
	}
	return nil
}

func validateSingleSlaveID(slaveIDs string) error {
	ids, err := routing.ParseSlaveIDs(slaveIDs)
	if err != nil {
		return fmt.Errorf("invalid slave_ids %q: %w", slaveIDs, err)
	}
	if len(ids) != 1 {
		return fmt.Errorf("slave_ids %q must resolve to exactly one ID, got %d", slaveIDs, len(ids))
	}
	if ids[0] < 1 || ids[0] > 247 {
		return fmt.Errorf("slave_ids %q must be in range 1..247, got %d", slaveIDs, ids[0])
	}
	return nil
}

// mappingTables validates the (source table, target table) pairing and
// returns the target table name, used as part of the overlap-tracking key.
func mappingTables(m MappingConfig) (targetTable string, err error) {
	switch {
	case m.Source.Table == "coils" && m.Target.Table == "discrete_inputs":
		return "discrete_inputs", nil
	case m.Source.Table == "holding_registers" && m.Target.Table == "input_registers":
		return "input_registers", nil
	default:
		return "", fmt.Errorf("invalid mapping table pairing %q -> %q (only coils->discrete_inputs and holding_registers->input_registers are allowed)", m.Source.Table, m.Target.Table)
	}
}

func validateMappings(ds DownstreamConfig, targetsBySimTable map[string][]addrRange) error {
	sourceRangesByTable := make(map[string][]addrRange, 2)

	for _, m := range ds.Mappings {
		targetTable, err := mappingTables(m)
		if err != nil {
			return err
		}
		if m.Source.Count == 0 {
			return fmt.Errorf("mapping count must be greater than 0")
		}

		srcEnd := uint32(m.Source.StartAddress) + uint32(m.Source.Count)
		if srcEnd > 65536 {
			return fmt.Errorf("mapping source range [%d,%d) is out of bounds", m.Source.StartAddress, srcEnd)
		}
		tgtEnd := uint32(m.Target.StartAddress) + uint32(m.Source.Count)
		if tgtEnd > 65536 {
			return fmt.Errorf("mapping target range [%d,%d) is out of bounds", m.Target.StartAddress, tgtEnd)
		}

		sr := addrRange{start: uint32(m.Source.StartAddress), end: srcEnd}
		for _, existing := range sourceRangesByTable[m.Source.Table] {
			if sr.overlaps(existing) {
				return fmt.Errorf("overlapping source mapping ranges in table %q", m.Source.Table)
			}
		}
		sourceRangesByTable[m.Source.Table] = append(sourceRangesByTable[m.Source.Table], sr)

		key := ds.SimulationRef + "|" + targetTable
		tr := addrRange{start: uint32(m.Target.StartAddress), end: tgtEnd}
		for _, existing := range targetsBySimTable[key] {
			if tr.overlaps(existing) {
				return fmt.Errorf("target range [%d,%d) in table %q overlaps another injector's mapping into simulation %q", m.Target.StartAddress, tgtEnd, targetTable, ds.SimulationRef)
			}
		}
		targetsBySimTable[key] = append(targetsBySimTable[key], tr)
	}

	return nil
}

func checkListenConflict(seen map[string]string, gatewayName string, us UpstreamConfig) error {
	var key string
	switch us.Type {
	case "tcp", "rtu-over-tcp":
		if us.Tcp.Address == "" {
			return nil
		}
		key = "tcp:" + us.Tcp.Address
	case "rtu":
		if us.Serial.Device == "" {
			return nil
		}
		key = "serial:" + us.Serial.Device
	default:
		return nil
	}

	if prevGateway, exists := seen[key]; exists {
		return fmt.Errorf("config: listen conflict on %s, used by both gateway %q and gateway %q", key, prevGateway, gatewayName)
	}
	seen[key] = gatewayName
	return nil
}
