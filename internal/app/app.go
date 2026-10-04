// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package app assembles the simulations and gateways described by a config,
// so the CLI and the desktop app start exactly the same runtime.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/internal/routing"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/internal/simulation/persistence"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
	"github.com/ffutop/modbus-gateway/internal/transport"
	"github.com/ffutop/modbus-gateway/internal/transport/injector"
	"github.com/ffutop/modbus-gateway/internal/transport/local"
	"github.com/ffutop/modbus-gateway/internal/transport/rtu"
	rtuovertcp "github.com/ffutop/modbus-gateway/internal/transport/rtu-over-tcp"
	"github.com/ffutop/modbus-gateway/internal/transport/tcp"
)

// App is the assembled runtime: open simulations and ready-to-start
// gateways.
type App struct {
	Gateways    []*gateway.Gateway
	Simulations map[string]*simulation.Simulation

	wg sync.WaitGroup
}

// New opens every simulation and builds every gateway in cfg, reporting
// events to recorder (which may be nil). A gateway whose downstreams or
// upstreams cannot be created is skipped with an error log; a bad routing
// table, a simulation that fails to open, or having no usable gateway at all
// fails the whole call, since a simulation may be shared across gateways.
func New(cfg *config.Config, recorder *telemetry.Recorder) (*App, error) {
	a := &App{Simulations: make(map[string]*simulation.Simulation, len(cfg.Simulations))}
	for _, simCfg := range cfg.Simulations {
		storage := persistence.New(persistence.Config{
			Type: simCfg.Persistence.Type,
			Path: simCfg.Persistence.Path,
		})
		sim, err := simulation.Open(simCfg.Name, storage)
		if err != nil {
			a.Close()
			return nil, fmt.Errorf("open simulation %q: %w", simCfg.Name, err)
		}
		a.Simulations[simCfg.Name] = sim
	}

	for _, gwCfg := range cfg.Gateways {
		gw, err := a.newGateway(gwCfg)
		if err != nil {
			a.Close()
			return nil, err
		}
		if gw == nil {
			continue
		}
		gw.Telemetry = recorder
		a.Gateways = append(a.Gateways, gw)
	}

	if len(a.Gateways) == 0 {
		a.Close()
		return nil, errors.New("no valid gateways configured")
	}
	return a, nil
}

// newGateway returns nil, nil when the gateway is skipped.
func (a *App) newGateway(gwCfg config.GatewayConfig) (*gateway.Gateway, error) {
	routes := make(map[byte]transport.Downstream)
	var defaultRoute transport.Downstream
	names := make(map[transport.Downstream]string)

	// Compatibility Check: If only one downstream and no SlaveIDs, treat as default route
	if len(gwCfg.Downstreams) == 1 && gwCfg.Downstreams[0].SlaveIDs == "" {
		ds, err := a.createDownstream(gwCfg.Downstreams[0])
		if err != nil {
			slog.Error("Failed to create default downstream", "gateway", gwCfg.Name, "err", err)
			return nil, nil
		}
		defaultRoute = ds
		names[ds] = gwCfg.Downstreams[0].DisplayName(0)
		slog.Info("Configured default route (legacy mode)", "gateway", gwCfg.Name)
	} else {
		// Routing Mode
		for i, dsCfg := range gwCfg.Downstreams {
			ds, err := a.createDownstream(dsCfg)
			if err != nil {
				slog.Error("Failed to create downstream", "gateway", gwCfg.Name, "err", err)
				continue
			}
			names[ds] = dsCfg.DisplayName(i)

			ids, err := routing.ParseSlaveIDs(dsCfg.SlaveIDs)
			if err != nil {
				return nil, fmt.Errorf("gateway %q: parse slave IDs %q: %w", gwCfg.Name, dsCfg.SlaveIDs, err)
			}

			if len(ids) == 0 {
				slog.Warn("Downstream configured without SlaveIDs in routing mode, it will be unreachable", "gateway", gwCfg.Name, "type", dsCfg.Type)
				continue
			}

			for _, id := range ids {
				if _, exists := routes[id]; exists {
					return nil, fmt.Errorf("gateway %q: duplicate route for slave ID %d", gwCfg.Name, id)
				}
				routes[id] = ds
			}
		}
		slog.Info("Configured routing table", "gateway", gwCfg.Name, "routes_count", len(routes))
	}

	if len(routes) == 0 && defaultRoute == nil {
		slog.Error("Gateway has no valid routes", "gateway", gwCfg.Name)
		return nil, nil
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
	gw.DownstreamNames = names
	return gw, nil
}

func (a *App) createDownstream(cfg config.DownstreamConfig) (transport.Downstream, error) {
	switch cfg.Type {
	case "tcp":
		return tcp.NewClient(cfg.Tcp.Address), nil
	case "rtu":
		return rtu.NewClient(cfg.Serial), nil
	case "rtu-over-tcp":
		return rtuovertcp.NewClient(cfg.Tcp.Address), nil
	case "local":
		sim, ok := a.Simulations[cfg.SimulationRef]
		if !ok {
			return nil, fmt.Errorf("simulation %q not found", cfg.SimulationRef)
		}
		return local.NewClient(sim), nil
	case "injector":
		sim, ok := a.Simulations[cfg.SimulationRef]
		if !ok {
			return nil, fmt.Errorf("simulation %q not found", cfg.SimulationRef)
		}
		return injector.NewClient(sim, cfg.Mappings), nil
	default:
		return nil, fmt.Errorf("unknown downstream type: %s", cfg.Type)
	}
}

// Start runs every gateway in the background until ctx is canceled; Wait
// blocks until they have all shut down.
func (a *App) Start(ctx context.Context) {
	for _, gw := range a.Gateways {
		a.wg.Add(1)
		go func(g *gateway.Gateway) {
			defer a.wg.Done()
			if err := g.Start(ctx); err != nil {
				slog.Error("Gateway stopped with error", "name", g.Name, "err", err)
			}
		}(gw)
	}
}

// Wait blocks until every gateway started by Start has returned.
func (a *App) Wait() { a.wg.Wait() }

// Close flushes and closes every simulation. Call it after Wait.
func (a *App) Close() {
	for name, sim := range a.Simulations {
		if err := sim.Close(); err != nil {
			slog.Error("Failed to close simulation", "simulation", name, "err", err)
		}
	}
}

// SortedSimulations returns the simulations ordered by name.
// UpstreamStatuses reports every upstream of every started gateway.
func (a *App) UpstreamStatuses() []gateway.UpstreamStatus {
	var out []gateway.UpstreamStatus
	for _, gw := range a.Gateways {
		out = append(out, gw.UpstreamStatuses()...)
	}
	return out
}

func (a *App) SortedSimulations() []*simulation.Simulation {
	sims := make([]*simulation.Simulation, 0, len(a.Simulations))
	for _, s := range a.Simulations {
		sims = append(sims, s)
	}
	sort.Slice(sims, func(i, j int) bool { return sims[i].Name < sims[j].Name })
	return sims
}
