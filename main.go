// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/transport"
	"github.com/ffutop/modbus-gateway/transport/rtu"
	"github.com/ffutop/modbus-gateway/transport/tcp"
)

func main() {
	configFile := flag.String("config", "", "Path to config file")
	flag.Parse()

	// Load Configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	setupLogger(cfg.Log)

	slog.Info("Starting Modbus Gateway...")

	// Create Gateways
	var gateways []*gateway.Gateway

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, gwCfg := range cfg.Gateways {
		// Setup Routing
		routes := make(map[byte]transport.Downstream)
		var defaultRoute transport.Downstream

		// Compatibility Check: If only one downstream and no SlaveIDs, treat as default route
		if len(gwCfg.Downstreams) == 1 && gwCfg.Downstreams[0].SlaveIDs == "" {
			ds, err := createDownstream(gwCfg.Downstreams[0])
			if err != nil {
				slog.Error("Failed to create default downstream", "gateway", gwCfg.Name, "err", err)
				continue
			}
			defaultRoute = ds
			slog.Info("Configured default route (legacy mode)", "gateway", gwCfg.Name)
		} else {
			// Routing Mode
			for _, dsCfg := range gwCfg.Downstreams {
				ds, err := createDownstream(dsCfg)
				if err != nil {
					slog.Error("Failed to create downstream", "gateway", gwCfg.Name, "err", err)
					continue
				}

				ids, err := gateway.ParseSlaveIDs(dsCfg.SlaveIDs)
				if err != nil {
					slog.Error("Failed to parse slave IDs", "gateway", gwCfg.Name, "slave_ids", dsCfg.SlaveIDs, "err", err)
					os.Exit(1)
				}

				if len(ids) == 0 {
					slog.Warn("Downstream configured without SlaveIDs in routing mode, it will be unreachable", "gateway", gwCfg.Name, "type", dsCfg.Type)
					continue
				}

				for _, id := range ids {
					if _, exists := routes[id]; exists {
						slog.Error("Duplicate route for slave ID", "id", id, "gateway", gwCfg.Name)
						os.Exit(1)
					}
					routes[id] = ds
				}
			}
			slog.Info("Configured routing table", "gateway", gwCfg.Name, "routes_count", len(routes))
		}

		if len(routes) == 0 && defaultRoute == nil {
			slog.Error("Gateway has no valid routes", "gateway", gwCfg.Name)
			continue
		}

		// Create Upstreams
		var upstreams []transport.Upstream
		for _, usCfg := range gwCfg.Upstreams {
			var us transport.Upstream
			switch usCfg.Type {
			case "tcp":
				us = tcp.NewServer(usCfg.Tcp.Address)
			case "rtu":
				us = rtu.NewServer(usCfg.Serial)
			default:
				slog.Error("Unknown upstream type", "type", usCfg.Type, "gateway", gwCfg.Name)
				continue
			}
			upstreams = append(upstreams, us)
		}

		gw := gateway.NewGateway(gwCfg.Name, upstreams, routes, defaultRoute)
		gateways = append(gateways, gw)
	}

	if len(gateways) == 0 {
		slog.Error("No valid gateways configured. Exiting.")
		os.Exit(1)
	}

	// Start Gateways
	var wg sync.WaitGroup
	for _, gw := range gateways {
		wg.Add(1)
		go func(g *gateway.Gateway) {
			defer wg.Done()
			if err := g.Start(ctx); err != nil {
				slog.Error("Gateway stopped with error", "name", g.Name, "err", err)
			}
		}(gw)
	}

	// Wait for Signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutting down...")
	cancel()
	wg.Wait()
	slog.Info("Goodbye.")
}

func createDownstream(cfg config.DownstreamConfig) (transport.Downstream, error) {
	switch cfg.Type {
	case "tcp":
		return tcp.NewClient(cfg.Tcp.Address), nil
	case "rtu":
		return rtu.NewClient(cfg.Serial), nil
	case "local":
		return nil, fmt.Errorf("local type not yet implemented")
	default:
		return nil, fmt.Errorf("unknown downstream type: %s", cfg.Type)
	}
}

func setupLogger(cfg config.LogConfig) {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	switch cfg.Level {
	case "debug":
		opts.Level = slog.LevelDebug
	case "warn":
		opts.Level = slog.LevelWarn
	case "error":
		opts.Level = slog.LevelError
	}

	var handler slog.Handler
	if cfg.File != "" && cfg.File != "-" {
		f, err := os.OpenFile(cfg.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("Failed to open log file, falling back to stdout: %v\n", err)
			handler = slog.NewTextHandler(os.Stdout, opts)
		} else {
			handler = slog.NewTextHandler(f, opts)
		}
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
