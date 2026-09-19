// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/internal/local-slave/persistence"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/transport"
	"github.com/ffutop/modbus-gateway/transport/injector"
	"github.com/ffutop/modbus-gateway/transport/local"
	"github.com/ffutop/modbus-gateway/transport/rtu"
	rtuovertcp "github.com/ffutop/modbus-gateway/transport/rtu-over-tcp"
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

	if cfg.Pprof.Enabled {
		addr := cfg.Pprof.Address
		if addr == "" {
			addr = "localhost:6060"
		}
		go func() {
			slog.Info("Starting pprof server", "addr", addr)
			if err := http.ListenAndServe(addr, nil); err != nil {
				slog.Error("Failed to start pprof server", "err", err)
			}
		}()
	}

	slog.Info("Starting Modbus Gateway...")

	// Open every shared simulation model up front. A simulation's
	// persistence must successfully open/restore before any downstream
	// referencing it is allowed to start; since a simulation may be shared
	// by downstreams across multiple gateways, any failure here aborts the
	// whole process rather than selectively starting a subset of gateways.
	simulations := make(map[string]*simulation.Simulation, len(cfg.Simulations))
	for _, simCfg := range cfg.Simulations {
		storage := persistence.New(persistence.Config{
			Type: simCfg.Persistence.Type,
			Path: simCfg.Persistence.Path,
		})
		sim, err := simulation.Open(simCfg.Name, storage)
		if err != nil {
			slog.Error("Failed to open simulation persistence", "simulation", simCfg.Name, "err", err)
			os.Exit(1)
		}
		simulations[simCfg.Name] = sim
	}

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
			ds, err := createDownstream(gwCfg.Downstreams[0], simulations)
			if err != nil {
				slog.Error("Failed to create default downstream", "gateway", gwCfg.Name, "err", err)
				continue
			}
			defaultRoute = ds
			slog.Info("Configured default route (legacy mode)", "gateway", gwCfg.Name)
		} else {
			// Routing Mode
			for _, dsCfg := range gwCfg.Downstreams {
				ds, err := createDownstream(dsCfg, simulations)
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
			case "rtu-over-tcp":
				us = rtuovertcp.NewServer(usCfg.Tcp.Address)
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

	for name, sim := range simulations {
		if err := sim.Close(); err != nil {
			slog.Error("Failed to close simulation", "simulation", name, "err", err)
		}
	}

	slog.Info("Goodbye.")
}

func createDownstream(cfg config.DownstreamConfig, simulations map[string]*simulation.Simulation) (transport.Downstream, error) {
	switch cfg.Type {
	case "tcp":
		return tcp.NewClient(cfg.Tcp.Address), nil
	case "rtu":
		return rtu.NewClient(cfg.Serial), nil
	case "rtu-over-tcp":
		return rtuovertcp.NewClient(cfg.Tcp.Address), nil
	case "local":
		sim, ok := simulations[cfg.SimulationRef]
		if !ok {
			return nil, fmt.Errorf("simulation %q not found", cfg.SimulationRef)
		}
		return local.NewClient(sim), nil
	case "injector":
		sim, ok := simulations[cfg.SimulationRef]
		if !ok {
			return nil, fmt.Errorf("simulation %q not found", cfg.SimulationRef)
		}
		return injector.NewClient(sim, cfg.Mappings), nil
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
