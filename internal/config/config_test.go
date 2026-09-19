// Copyright (c) 2025-2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadYAML(t *testing.T, yaml string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return LoadConfig(path)
}

func mustLoadYAML(t *testing.T, yaml string) *Config {
	t.Helper()
	cfg, err := loadYAML(t, yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return cfg
}

// --- v0 (legacy) compatibility ---

func TestV0_NoPersistence_ParsesAndSynthesizesMemorySimulation(t *testing.T) {
	cfg := mustLoadYAML(t, `
gateways:
  - name: "local-gateway"
    upstreams:
      - type: "tcp"
        tcp:
          address: "0.0.0.0:33503"
    downstreams:
      - name: "local-device"
        type: "local"
        slave_ids: "1"
log:
  level: "debug"
`)
	if cfg.Version != 0 {
		t.Errorf("expected version 0, got %d", cfg.Version)
	}
	if len(cfg.Simulations) != 1 {
		t.Fatalf("expected 1 synthesized simulation, got %d", len(cfg.Simulations))
	}
	ds := cfg.Gateways[0].Downstreams[0]
	if ds.SimulationRef == "" || ds.SimulationRef != cfg.Simulations[0].Name {
		t.Errorf("expected downstream to reference the synthesized simulation, got ref=%q sim=%q", ds.SimulationRef, cfg.Simulations[0].Name)
	}
}

func TestV0_WithFilePersistence_PreservesPersistenceConfig(t *testing.T) {
	cfg := mustLoadYAML(t, `
gateways:
  - name: "persist-gw"
    upstreams:
      - type: "tcp"
        tcp:
          address: "0.0.0.0:33504"
    downstreams:
      - name: "local-db"
        type: "local"
        slave_ids: "1"
        local:
          persistence:
            type: "file"
            path: "/tmp/whatever.bin"
log:
  level: "debug"
`)
	if len(cfg.Simulations) != 1 {
		t.Fatalf("expected 1 simulation, got %d", len(cfg.Simulations))
	}
	sim := cfg.Simulations[0]
	if sim.Persistence.Type != "file" || sim.Persistence.Path != "/tmp/whatever.bin" {
		t.Errorf("expected persistence to be preserved, got %+v", sim.Persistence)
	}
}

func TestV0_MultipleLocalDownstreams_GetIndependentSimulations(t *testing.T) {
	cfg := mustLoadYAML(t, `
gateways:
  - name: "gw"
    upstreams:
      - type: "tcp"
        tcp:
          address: "0.0.0.0:33505"
    downstreams:
      - name: "d1"
        type: "local"
        slave_ids: "1"
      - name: "d2"
        type: "local"
        slave_ids: "2"
`)
	if len(cfg.Simulations) != 2 {
		t.Fatalf("expected 2 independent simulations (no implicit sharing in v0), got %d", len(cfg.Simulations))
	}
	if cfg.Gateways[0].Downstreams[0].SimulationRef == cfg.Gateways[0].Downstreams[1].SimulationRef {
		t.Error("expected v0 local downstreams to NOT share a simulation")
	}
}

func TestV0_TcpAndRtuDownstreams_StillWork(t *testing.T) {
	cfg := mustLoadYAML(t, `
gateways:
  - name: "gw"
    upstreams:
      - type: "tcp"
        tcp:
          address: "0.0.0.0:33506"
    downstreams:
      - name: "rtu-devices"
        type: "rtu"
        slave_ids: "1-10"
        serial:
          device: "/dev/ttyUSB1"
          baud_rate: 9600
          data_bits: 8
          parity: "n"
          stop_bits: 1
      - name: "tcp-device"
        type: "tcp"
        slave_ids: "20"
        tcp:
          address: "192.168.1.20:502"
`)
	if len(cfg.Gateways[0].Downstreams) != 2 {
		t.Fatalf("expected 2 downstreams, got %d", len(cfg.Gateways[0].Downstreams))
	}
	if cfg.Gateways[0].Downstreams[0].Serial.Parity != "N" {
		t.Errorf("expected fixupSerial to uppercase parity, got %q", cfg.Gateways[0].Downstreams[0].Serial.Parity)
	}
	if cfg.Gateways[0].Downstreams[0].Serial.Timeout == 0 {
		t.Errorf("expected fixupSerial to apply default timeout")
	}
}

// --- v1 acceptance ---

const v1ValidConfig = `
version: 1

simulations:
  - name: sim-device
    persistence:
      type: mmap
      path: /tmp/sim-device.bin

gateways:
  - name: business-modbus
    upstreams:
      - type: tcp
        tcp: { address: "0.0.0.0:34001" }
    downstreams:
      - type: local
        slave_ids: "100"
        simulation: { ref: sim-device }

  - name: simulation-injection
    upstreams:
      - type: tcp
        tcp: { address: "127.0.0.1:34002" }
    downstreams:
      - type: injector
        slave_ids: "247"
        simulation:
          ref: sim-device
          mappings:
            - source: { table: coils, start_address: 0, count: 256 }
              target: { table: discrete_inputs, start_address: 0 }
            - source: { table: holding_registers, start_address: 0, count: 1024 }
              target: { table: input_registers, start_address: 0 }

  - name: field-devices
    upstreams:
      - type: rtu-over-tcp
        tcp: { address: "0.0.0.0:34003" }
    downstreams:
      - name: rtu-devices
        type: rtu
        slave_ids: "1-10"
        serial: { device: "/dev/ttyUSB1", baud_rate: 9600, data_bits: 8, parity: "N", stop_bits: 1 }
      - name: tcp-device
        type: tcp
        slave_ids: "20"
        tcp: { address: "192.168.1.20:502" }

log:
  level: info
`

func TestV1_ValidSharedSimulationConfig_Parses(t *testing.T) {
	cfg := mustLoadYAML(t, v1ValidConfig)

	if cfg.Version != 1 {
		t.Fatalf("expected version 1, got %d", cfg.Version)
	}
	if len(cfg.Simulations) != 1 || cfg.Simulations[0].Name != "sim-device" {
		t.Fatalf("expected simulation 'sim-device', got %+v", cfg.Simulations)
	}

	localDS := cfg.Gateways[0].Downstreams[0]
	if localDS.Type != "local" || localDS.SimulationRef != "sim-device" {
		t.Errorf("expected local downstream to reference sim-device, got %+v", localDS)
	}

	injectorDS := cfg.Gateways[1].Downstreams[0]
	if injectorDS.Type != "injector" || injectorDS.SimulationRef != "sim-device" {
		t.Errorf("expected injector downstream to reference sim-device, got %+v", injectorDS)
	}
	if len(injectorDS.Mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(injectorDS.Mappings))
	}
	m0 := injectorDS.Mappings[0]
	if m0.Source.Table != "coils" || m0.Source.Count != 256 || m0.Target.Table != "discrete_inputs" {
		t.Errorf("unexpected mapping[0]: %+v", m0)
	}
}

// --- rejection cases ---

func TestConfig_RejectionCases(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "unknown version",
			yaml: `
version: 2
gateways: []
`,
		},
		{
			name: "non integer version",
			yaml: `
version: "abc"
gateways: []
`,
		},
		{
			name: "v0 with simulations block",
			yaml: `
simulations:
  - name: sim
    persistence: { type: memory }
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34101" } }]
    downstreams:
      - type: local
        slave_ids: "1"
`,
		},
		{
			name: "v0 with downstream simulation ref",
			yaml: `
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34102" } }]
    downstreams:
      - type: local
        slave_ids: "1"
        simulation: { ref: sim }
`,
		},
		{
			name: "v0 with injector downstream type",
			yaml: `
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34103" } }]
    downstreams:
      - type: injector
        slave_ids: "247"
`,
		},
		{
			name: "v1 unknown top level field",
			yaml: `
version: 1
totally_unknown_field: true
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34104" } }]
    downstreams:
      - type: local
        slave_ids: "1"
        simulation: { ref: sim }
`,
		},
		{
			name: "v1 unknown downstream field",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34105" } }]
    downstreams:
      - type: local
        slave_ids: "1"
        simulation: { ref: sim }
        local: { persistence: { type: memory } }
`,
		},
		{
			name: "v1 duplicate simulation name",
			yaml: `
version: 1
simulations:
  - name: sim
    persistence: { type: memory }
  - name: sim
    persistence: { type: memory }
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34106" } }]
    downstreams:
      - type: local
        slave_ids: "1"
        simulation: { ref: sim }
`,
		},
		{
			name: "v1 unknown simulation ref",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34107" } }]
    downstreams:
      - type: local
        slave_ids: "1"
        simulation: { ref: does-not-exist }
`,
		},
		{
			name: "v1 local with multiple slave_ids",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34108" } }]
    downstreams:
      - type: local
        slave_ids: "1,2"
        simulation: { ref: sim }
`,
		},
		{
			name: "v1 local slave id out of 1..247",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34109" } }]
    downstreams:
      - type: local
        slave_ids: "248"
        simulation: { ref: sim }
`,
		},
		{
			name: "v1 injector missing mappings",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34110" } }]
    downstreams:
      - type: injector
        slave_ids: "247"
        simulation: { ref: sim }
`,
		},
		{
			name: "v1 local with mappings",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34111" } }]
    downstreams:
      - type: local
        slave_ids: "1"
        simulation:
          ref: sim
          mappings:
            - source: { table: coils, start_address: 0, count: 1 }
              target: { table: discrete_inputs, start_address: 0 }
`,
		},
		{
			name: "v1 mapping bad table pairing",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34112" } }]
    downstreams:
      - type: injector
        slave_ids: "247"
        simulation:
          ref: sim
          mappings:
            - source: { table: coils, start_address: 0, count: 1 }
              target: { table: input_registers, start_address: 0 }
`,
		},
		{
			name: "v1 mapping count zero",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34113" } }]
    downstreams:
      - type: injector
        slave_ids: "247"
        simulation:
          ref: sim
          mappings:
            - source: { table: coils, start_address: 0, count: 0 }
              target: { table: discrete_inputs, start_address: 0 }
`,
		},
		{
			name: "v1 mapping source out of bounds",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34114" } }]
    downstreams:
      - type: injector
        slave_ids: "247"
        simulation:
          ref: sim
          mappings:
            - source: { table: coils, start_address: 65535, count: 2 }
              target: { table: discrete_inputs, start_address: 0 }
`,
		},
		{
			name: "v1 overlapping source ranges within one injector",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34115" } }]
    downstreams:
      - type: injector
        slave_ids: "247"
        simulation:
          ref: sim
          mappings:
            - source: { table: coils, start_address: 0, count: 10 }
              target: { table: discrete_inputs, start_address: 0 }
            - source: { table: coils, start_address: 5, count: 10 }
              target: { table: discrete_inputs, start_address: 100 }
`,
		},
		{
			name: "v1 overlapping target ranges across two injectors sharing a simulation",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw1
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34116" } }]
    downstreams:
      - type: injector
        slave_ids: "247"
        simulation:
          ref: sim
          mappings:
            - source: { table: coils, start_address: 0, count: 10 }
              target: { table: discrete_inputs, start_address: 0 }
  - name: gw2
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34117" } }]
    downstreams:
      - type: injector
        slave_ids: "246"
        simulation:
          ref: sim
          mappings:
            - source: { table: coils, start_address: 100, count: 10 }
              target: { table: discrete_inputs, start_address: 5 }
`,
		},
		{
			name: "v1 duplicate upstream listen address across gateways",
			yaml: `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw1
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34118" } }]
    downstreams:
      - type: local
        slave_ids: "1"
        simulation: { ref: sim }
  - name: gw2
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34118" } }]
    downstreams:
      - type: local
        slave_ids: "2"
        simulation: { ref: sim }
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadYAML(t, tt.yaml)
			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestConfig_NonOverlappingTargetRangesAcrossInjectors_Accepted(t *testing.T) {
	mustLoadYAML(t, `
version: 1
simulations: [{ name: sim, persistence: { type: memory } }]
gateways:
  - name: gw1
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34201" } }]
    downstreams:
      - type: injector
        slave_ids: "247"
        simulation:
          ref: sim
          mappings:
            - source: { table: coils, start_address: 0, count: 10 }
              target: { table: discrete_inputs, start_address: 0 }
  - name: gw2
    upstreams: [{ type: tcp, tcp: { address: "0.0.0.0:34202" } }]
    downstreams:
      - type: injector
        slave_ids: "246"
        simulation:
          ref: sim
          mappings:
            - source: { table: coils, start_address: 100, count: 10 }
              target: { table: discrete_inputs, start_address: 10 }
`)
}
