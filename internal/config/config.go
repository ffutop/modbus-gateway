// Copyright (c) 2025-2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cast"
	"github.com/spf13/viper"
)

// --- Normalized config, the only shape the rest of the program sees. ---

// Config defines the global configuration structure, after normalizing away
// the version-specific parsing rules (see LoadConfig).
type Config struct {
	Version     int
	Pprof       PprofConfig
	Log         LogConfig
	Simulations []SimulationConfig
	Gateways    []GatewayConfig
}

// SimulationConfig defines a named, shared simulated data model.
// In a v1 config it is declared explicitly under `simulations:`. In a v0
// config, LoadConfig synthesizes one per legacy `local` downstream so that
// today's "one local downstream = one independent model" behavior is
// preserved exactly.
type SimulationConfig struct {
	Name        string            `mapstructure:"name"`
	Persistence PersistenceConfig `mapstructure:"persistence"`
}

// GatewayConfig defines a single gateway instance
type GatewayConfig struct {
	Name        string
	Upstreams   []UpstreamConfig
	Downstreams []DownstreamConfig
}

// UpstreamConfig defines a master connecting to the gateway
type UpstreamConfig struct {
	Type   string       `mapstructure:"type"`   // "tcp", "rtu", "rtu-over-tcp"
	Tcp    TcpConfig    `mapstructure:"tcp"`    // Used if Type is "tcp" or "rtu-over-tcp"
	Serial SerialConfig `mapstructure:"serial"` // Used if Type is "rtu"
}

// DownstreamConfig defines the slave the gateway connects to
type DownstreamConfig struct {
	Name     string
	Type     string // "tcp", "rtu", "rtu-over-tcp", "local", "injector"
	SlaveIDs string
	Tcp      TcpConfig
	Serial   SerialConfig

	// SimulationRef names the SimulationConfig this "local"/"injector"
	// downstream is bound to. Always resolved (never a raw user string
	// that still needs lookup) once LoadConfig returns.
	SimulationRef string
	// Mappings is only set for "injector" downstreams.
	Mappings []MappingConfig
}

// MappingConfig maps a standard Modbus write source range to a target range
// in the shared simulation model. Only used by "injector" downstreams.
type MappingConfig struct {
	Source MappingSourceConfig `mapstructure:"source"`
	Target MappingTargetConfig `mapstructure:"target"`
}

// MappingSourceConfig is the standard Modbus write range an injector accepts.
type MappingSourceConfig struct {
	Table        string `mapstructure:"table"` // "coils" or "holding_registers"
	StartAddress uint16 `mapstructure:"start_address"`
	Count        uint16 `mapstructure:"count"`
}

// MappingTargetConfig is the simulation range a mapping writes into.
type MappingTargetConfig struct {
	Table        string `mapstructure:"table"` // "discrete_inputs" or "input_registers"
	StartAddress uint16 `mapstructure:"start_address"`
}

// LocalConfig defines settings for a v0 legacy local modbus slave device.
// Not part of the v1 schema: v1 simulations are declared under `simulations:`
// and referenced via `simulation.ref`.
type LocalConfig struct {
	Device      string            `mapstructure:"device"`
	Persistence PersistenceConfig `mapstructure:"persistence"`
}

// PersistenceConfig defines data storage settings
type PersistenceConfig struct {
	Type string `mapstructure:"type"` // "memory", "file", "mmap", "sql"
	Path string `mapstructure:"path"` // File path for "file/mmap" type
}

// TcpConfig defines TCP settings
type TcpConfig struct {
	Address string `mapstructure:"address"` // e.g. "0.0.0.0:502" or "192.168.1.100:502"
}

// SerialConfig defines RTU settings
type SerialConfig struct {
	Device    string        `mapstructure:"device"`
	BaudRate  int           `mapstructure:"baud_rate"`
	DataBits  int           `mapstructure:"data_bits"`
	Parity    string        `mapstructure:"parity"`
	StopBits  int           `mapstructure:"stop_bits"`
	Timeout   time.Duration `mapstructure:"timeout"`
	RqstPause time.Duration `mapstructure:"rqst_pause"` // Pause between requests

	// RS485 specific
	RS485              bool          `mapstructure:"rs485"`
	DelayRtsBeforeSend time.Duration `mapstructure:"delay_rts_before_send"`
	DelayRtsAfterSend  time.Duration `mapstructure:"delay_rts_after_send"`
	RtsHighDuringSend  bool          `mapstructure:"rts_high_during_send"`
	RtsHighAfterSend   bool          `mapstructure:"rts_high_after_send"`
	RxDuringTx         bool          `mapstructure:"rx_during_tx"`
}

// LogConfig defines logging configuration
type LogConfig struct {
	Level string `mapstructure:"level"` // debug, info, warn, error
	File  string `mapstructure:"file"`  // Log file path
}

// PprofConfig defines pprof configuration
type PprofConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Address string `mapstructure:"address"` // e.g. "localhost:6060"
}

// --- Raw (version-specific) decode shapes. Never seen outside this file. ---

type rawV0Config struct {
	Gateways []rawV0Gateway `mapstructure:"gateways"`
	Log      LogConfig      `mapstructure:"log"`
	Pprof    PprofConfig    `mapstructure:"pprof"`
}

type rawV0Gateway struct {
	Name        string            `mapstructure:"name"`
	Upstreams   []UpstreamConfig  `mapstructure:"upstreams"`
	Downstreams []rawV0Downstream `mapstructure:"downstreams"`
}

type rawV0Downstream struct {
	Name     string       `mapstructure:"name"`
	Type     string       `mapstructure:"type"`
	SlaveIDs string       `mapstructure:"slave_ids"`
	Tcp      TcpConfig    `mapstructure:"tcp"`
	Serial   SerialConfig `mapstructure:"serial"`
	Local    LocalConfig  `mapstructure:"local"`
}

type rawV1Config struct {
	Version     int                `mapstructure:"version"`
	Pprof       PprofConfig        `mapstructure:"pprof"`
	Log         LogConfig          `mapstructure:"log"`
	Simulations []SimulationConfig `mapstructure:"simulations"`
	Gateways    []rawV1Gateway     `mapstructure:"gateways"`
}

type rawV1Gateway struct {
	Name        string            `mapstructure:"name"`
	Upstreams   []UpstreamConfig  `mapstructure:"upstreams"`
	Downstreams []rawV1Downstream `mapstructure:"downstreams"`
}

type rawV1Downstream struct {
	Name       string            `mapstructure:"name"`
	Type       string            `mapstructure:"type"`
	SlaveIDs   string            `mapstructure:"slave_ids"`
	Tcp        TcpConfig         `mapstructure:"tcp"`
	Serial     SerialConfig      `mapstructure:"serial"`
	Simulation *rawSimulationRef `mapstructure:"simulation"`
}

type rawSimulationRef struct {
	Ref      string          `mapstructure:"ref"`
	Mappings []MappingConfig `mapstructure:"mappings"`
}

// LoadConfig loads configuration from file, choosing the v0 (legacy, no
// `version` field) or v1 (`version: 1`, shared simulation contract) parsing
// rules, and returns a fully normalized and validated Config.
func LoadConfig(configFile string) (*Config, error) {
	v := viper.New()

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("/etc/modbusgw/")
		v.AddConfigPath("$HOME/.modbusgw")
		v.AddConfigPath(".")
	}

	// Set defaults
	v.SetDefault("log.level", "info")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to found config file: %w", err)
		}

		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	version, err := resolveVersion(v)
	if err != nil {
		return nil, err
	}

	var cfg *Config
	switch version {
	case 0:
		cfg, err = loadV0(v)
	case 1:
		cfg, err = loadV1(v)
	default:
		return nil, fmt.Errorf("config: unsupported version %d (supported: 0, 1)", version)
	}
	if err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func resolveVersion(v *viper.Viper) (int, error) {
	if !v.IsSet("version") {
		return 0, nil
	}
	version, err := cast.ToIntE(v.Get("version"))
	if err != nil {
		return 0, fmt.Errorf("config: 'version' must be an integer: %w", err)
	}
	return version, nil
}

// loadV0 parses the historical, lenient schema: unknown fields are ignored
// (as they always have been), but any v1-only field is explicitly rejected
// since it would otherwise be silently dropped, hiding a likely config
// mistake (forgetting `version: 1`).
func loadV0(v *viper.Viper) (*Config, error) {
	if err := rejectV1Fields(v); err != nil {
		return nil, err
	}

	var raw rawV0Config
	if err := v.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg := &Config{Version: 0, Pprof: raw.Pprof, Log: raw.Log}

	for gi := range raw.Gateways {
		rg := &raw.Gateways[gi]
		for ui := range rg.Upstreams {
			fixupSerial(&rg.Upstreams[ui].Serial)
		}

		gateway := GatewayConfig{Name: rg.Name, Upstreams: rg.Upstreams}

		for di := range rg.Downstreams {
			rd := &rg.Downstreams[di]
			fixupSerial(&rd.Serial)

			downstream := DownstreamConfig{
				Name:     rd.Name,
				Type:     rd.Type,
				SlaveIDs: rd.SlaveIDs,
				Tcp:      rd.Tcp,
				Serial:   rd.Serial,
			}

			if rd.Type == "local" {
				// Every v0 `local` downstream keeps creating its own
				// independent model, exactly like today - just modeled now
				// as an (internal, unnamed) Simulation.
				simName := fmt.Sprintf("__v0:%s#%d", rg.Name, di)
				cfg.Simulations = append(cfg.Simulations, SimulationConfig{
					Name:        simName,
					Persistence: rd.Local.Persistence,
				})
				downstream.SimulationRef = simName
			}

			gateway.Downstreams = append(gateway.Downstreams, downstream)
		}

		cfg.Gateways = append(cfg.Gateways, gateway)
	}

	return cfg, nil
}

// rejectV1Fields inspects the raw config tree (before struct decoding, since
// the v0 struct shape does not even have these fields) for anything that
// only makes sense under `version: 1`.
func rejectV1Fields(v *viper.Viper) error {
	if v.IsSet("simulations") {
		return fmt.Errorf("config: 'simulations' requires 'version: 1'")
	}

	gatewaysRaw, ok := v.Get("gateways").([]interface{})
	if !ok {
		return nil
	}
	for _, gRaw := range gatewaysRaw {
		gMap, ok := gRaw.(map[string]interface{})
		if !ok {
			continue
		}
		downstreamsRaw, ok := gMap["downstreams"].([]interface{})
		if !ok {
			continue
		}
		for _, dRaw := range downstreamsRaw {
			dMap, ok := dRaw.(map[string]interface{})
			if !ok {
				continue
			}
			if _, has := dMap["simulation"]; has {
				return fmt.Errorf("config: downstream field 'simulation' requires 'version: 1'")
			}
			if t, _ := dMap["type"].(string); t == "injector" {
				return fmt.Errorf("config: downstream type 'injector' requires 'version: 1'")
			}
		}
	}
	return nil
}

// loadV1 parses the shared simulation schema with strict unknown-field
// rejection.
func loadV1(v *viper.Viper) (*Config, error) {
	var raw rawV1Config
	if err := v.UnmarshalExact(&raw); err != nil {
		return nil, fmt.Errorf("config: failed to parse v1 config: %w", err)
	}

	cfg := &Config{
		Version:     1,
		Pprof:       raw.Pprof,
		Log:         raw.Log,
		Simulations: raw.Simulations,
	}

	for gi := range raw.Gateways {
		rg := &raw.Gateways[gi]
		for ui := range rg.Upstreams {
			fixupSerial(&rg.Upstreams[ui].Serial)
		}

		gateway := GatewayConfig{Name: rg.Name, Upstreams: rg.Upstreams}

		for di := range rg.Downstreams {
			rd := &rg.Downstreams[di]
			fixupSerial(&rd.Serial)

			downstream := DownstreamConfig{
				Name:     rd.Name,
				Type:     rd.Type,
				SlaveIDs: rd.SlaveIDs,
				Tcp:      rd.Tcp,
				Serial:   rd.Serial,
			}
			if rd.Simulation != nil {
				downstream.SimulationRef = rd.Simulation.Ref
				downstream.Mappings = rd.Simulation.Mappings
			}

			gateway.Downstreams = append(gateway.Downstreams, downstream)
		}

		cfg.Gateways = append(cfg.Gateways, gateway)
	}

	return cfg, nil
}

func fixupSerial(s *SerialConfig) {
	s.Parity = strings.ToUpper(s.Parity)
	if s.Timeout == 0 {
		s.Timeout = 500 * time.Millisecond
	}
	if s.RqstPause == 0 {
		s.RqstPause = 100 * time.Millisecond
	}
}
