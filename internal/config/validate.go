// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package config

import (
	"fmt"
	"net"
	"strconv"

	"github.com/ffutop/modbus-gateway/internal/routing"
)

// addrRange is a half-open [start, end) range over the 0..65535 address space.
type addrRange struct {
	start, end uint32
}

func (r addrRange) overlaps(o addrRange) bool {
	return r.start < o.end && o.start < r.end
}

// Problem is one rule violation, located by its path in the config file
// (e.g. ["gateways", 0, "downstreams", 1, "slave_ids"]), so an editor can
// point at the offending field.
type Problem struct {
	Path    []any  `json:"path"`
	Message string `json:"message"`
}

// Validate checks cross-cutting rules that can only be enforced once the
// whole config has been parsed and normalized, and returns the first
// violation. See Problems for the full list.
func (c *Config) Validate() error {
	if p := c.Problems(); len(p) > 0 {
		return fmt.Errorf("%s", p[0].Message)
	}
	return nil
}

// Problems checks simulation references, slave ID routing (each ID routes to
// at most one downstream per gateway), mapping validity/overlap (both within
// an injector and across every injector sharing a simulation), single-slave-ID
// entries, and (v1 only) duplicate upstream listen addresses. It runs for
// both v0 and v1 configs; v0 configs simply never exercise the
// injector/mapping branches.
func (c *Config) Problems() []Problem {
	var problems []Problem
	add := func(path []any, format string, args ...any) {
		problems = append(problems, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	simNames := make(map[string]bool, len(c.Simulations))
	for si, s := range c.Simulations {
		path := []any{"simulations", si, "name"}
		if s.Name == "" {
			add(path, "config: simulation name must not be empty")
			continue
		}
		if simNames[s.Name] {
			add(path, "config: duplicate simulation name %q", s.Name)
		}
		simNames[s.Name] = true
	}

	// Target ranges, keyed by "<simulation>|<table>", accumulated across
	// every injector downstream (possibly in different gateways) that
	// references the same simulation.
	targetsBySimTable := make(map[string][]addrRange)

	// Upstream listen addresses/devices, v1 only.
	listenSeen := make(map[string]string)

	for gi, gw := range c.Gateways {
		if c.Version == 1 {
			for ui, us := range gw.Upstreams {
				if err := checkListenConflict(listenSeen, gw.Name, us); err != nil {
					add([]any{"gateways", gi, "upstreams", ui}, "%v", err)
				}
			}
		}

		// A lone downstream without slave_ids is the legacy default route.
		legacyDefault := len(gw.Downstreams) == 1 && gw.Downstreams[0].SlaveIDs == ""
		routedBy := make(map[byte]string)

		for di, ds := range gw.Downstreams {
			dsPath := func(field ...any) []any { return append([]any{"gateways", gi, "downstreams", di}, field...) }
			prefix := fmt.Sprintf("gateway %q downstream %q: ", gw.Name, ds.Name)

			if !legacyDefault && ds.SlaveIDs != "" {
				ids, err := routing.ParseSlaveIDs(ds.SlaveIDs)
				if err != nil {
					add(dsPath("slave_ids"), prefix+"invalid slave_ids %q: %v", ds.SlaveIDs, err)
				}
				for _, id := range ids {
					if other, taken := routedBy[id]; taken {
						add(dsPath("slave_ids"), prefix+"slave ID %d is already routed to downstream %q", id, other)
						break
					}
					routedBy[id] = ds.Name
				}
			}

			switch ds.Type {
			case "local":
				if err := validateSimRef(simNames, ds.SimulationRef); err != nil {
					add(dsPath("simulation", "ref"), prefix+"%v", err)
				}
				if c.Version == 1 {
					if len(ds.Mappings) > 0 {
						add(dsPath("simulation", "mappings"), prefix+"'local' must not declare simulation.mappings")
					}
					if err := validateSingleSlaveID(ds.SlaveIDs); err != nil {
						add(dsPath("slave_ids"), prefix+"%v", err)
					}
				}

			case "injector":
				if c.Version != 1 {
					add(dsPath("type"), prefix+"downstream type 'injector' requires version: 1")
					continue
				}
				if err := validateSimRef(simNames, ds.SimulationRef); err != nil {
					add(dsPath("simulation", "ref"), prefix+"%v", err)
				}
				if err := validateSingleSlaveID(ds.SlaveIDs); err != nil {
					add(dsPath("slave_ids"), prefix+"%v", err)
				}
				if len(ds.Mappings) == 0 {
					add(dsPath("simulation", "mappings"), prefix+"'injector' requires at least one mapping")
				}
				if mi, err := validateMappings(ds, targetsBySimTable); err != nil {
					add(dsPath("simulation", "mappings", mi), prefix+"%v", err)
				}

			default:
				// tcp/rtu/rtu-over-tcp: no simulation-specific rules.
			}
		}
	}

	if c.UI.Enabled {
		if err := checkUIListen(c.UI.Address(), c.Gateways); err != nil {
			add([]any{"ui", "listen"}, "config: ui.listen: %v", err)
		}
	}

	return problems
}

// checkUIListen requires a host:port the management API can bind that does
// not collide with any Modbus TCP listener (a wildcard host collides with
// every host on the same port).
func checkUIListen(addr string, gateways []GatewayConfig) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%q is not host:port", addr)
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return fmt.Errorf("%q has an invalid port", addr)
	}
	if port == 0 {
		return nil
	}
	for _, gw := range gateways {
		for _, us := range gw.Upstreams {
			if us.Type != "tcp" && us.Type != "rtu-over-tcp" {
				continue
			}
			uHost, uPort, err := net.SplitHostPort(us.Tcp.Address)
			if err != nil || uPort != portStr {
				continue
			}
			if host == uHost || isWildcardHost(host) || isWildcardHost(uHost) {
				return fmt.Errorf("%s collides with gateway %q upstream %s", addr, gw.Name, us.Tcp.Address)
			}
		}
	}
	return nil
}

func isWildcardHost(h string) bool {
	return h == "" || h == "0.0.0.0" || h == "::"
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

// validateMappings returns the index of the first invalid mapping with its
// error.
func validateMappings(ds DownstreamConfig, targetsBySimTable map[string][]addrRange) (int, error) {
	sourceRangesByTable := make(map[string][]addrRange, 2)

	for mi, m := range ds.Mappings {
		targetTable, err := mappingTables(m)
		if err != nil {
			return mi, err
		}
		if m.Source.Count == 0 {
			return mi, fmt.Errorf("mapping count must be greater than 0")
		}

		srcEnd := uint32(m.Source.StartAddress) + uint32(m.Source.Count)
		if srcEnd > 65536 {
			return mi, fmt.Errorf("mapping source range [%d,%d) is out of bounds", m.Source.StartAddress, srcEnd)
		}
		tgtEnd := uint32(m.Target.StartAddress) + uint32(m.Source.Count)
		if tgtEnd > 65536 {
			return mi, fmt.Errorf("mapping target range [%d,%d) is out of bounds", m.Target.StartAddress, tgtEnd)
		}

		sr := addrRange{start: uint32(m.Source.StartAddress), end: srcEnd}
		for _, existing := range sourceRangesByTable[m.Source.Table] {
			if sr.overlaps(existing) {
				return mi, fmt.Errorf("overlapping source mapping ranges in table %q", m.Source.Table)
			}
		}
		sourceRangesByTable[m.Source.Table] = append(sourceRangesByTable[m.Source.Table], sr)

		key := ds.SimulationRef + "|" + targetTable
		tr := addrRange{start: uint32(m.Target.StartAddress), end: tgtEnd}
		for _, existing := range targetsBySimTable[key] {
			if tr.overlaps(existing) {
				return mi, fmt.Errorf("target range [%d,%d) in table %q overlaps another injector's mapping into simulation %q", m.Target.StartAddress, tgtEnd, targetTable, ds.SimulationRef)
			}
		}
		targetsBySimTable[key] = append(targetsBySimTable[key], tr)
	}

	return 0, nil
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
