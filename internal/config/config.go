// Copyright (c) 2025-2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config defines the global configuration structure
type Config struct {
	Gateways []GatewayConfig `mapstructure:"gateways"`
	Log      LogConfig       `mapstructure:"log"`
}

// LogConfig defines logging configuration
type LogConfig struct {
	Level string `mapstructure:"level"` // debug, info, warn, error
	File  string `mapstructure:"file"`  // Log file path
}

// GatewayConfig defines a single gateway instance
type GatewayConfig struct {
	Name        string             `mapstructure:"name"`
	Upstreams   []UpstreamConfig   `mapstructure:"upstreams"`
	Downstreams []DownstreamConfig `mapstructure:"downstreams"`
}

// UpstreamConfig defines a master connecting to the gateway
type UpstreamConfig struct {
	Type   string       `mapstructure:"type"`   // "tcp", "rtu", "rtu-over-tcp"
	Tcp    TcpConfig    `mapstructure:"tcp"`    // Used if Type is "tcp" or "rtu-over-tcp"
	Serial SerialConfig `mapstructure:"serial"` // Used if Type is "rtu"
}

// DownstreamConfig defines the slave the gateway connects to
type DownstreamConfig struct {
	Name     string       `mapstructure:"name"`      // Optional name for logging
	Type     string       `mapstructure:"type"`      // "tcp", "rtu", "rtu-over-tcp", "local"
	SlaveIDs string       `mapstructure:"slave_ids"` // Routing rules: "1", "1,2", "1-10"
	Tcp      TcpConfig    `mapstructure:"tcp"`       // Used if Type is "tcp" or "rtu-over-tcp"
	Serial   SerialConfig `mapstructure:"serial"`    // Used if Type is "rtu"
	Local    LocalConfig  `mapstructure:"local"`     // Used if Type is "local"
}

// LocalConfig defines settings for local modbus slave device
type LocalConfig struct {
	Device      string            `mapstructure:"device"`
	Persistence PersistenceConfig `mapstructure:"persistence"`
}

// PersistenceConfig defines data storage settings
type PersistenceConfig struct {
	Type string `mapstructure:"type"` // "memory", "file", "mmap"
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

// LoadConfig loads configuration from file
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

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate / Fixups
	for i := range config.Gateways {
		gw := &config.Gateways[i]

		for j := range gw.Downstreams {
			fixupSerial(&gw.Downstreams[j].Serial)
		}

		for j := range gw.Upstreams {
			fixupSerial(&gw.Upstreams[j].Serial)
		}
	}

	return &config, nil
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