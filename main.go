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
		// Create Downstream
		var ds transport.Downstream
		switch gwCfg.Downstream.Type {
		case "tcp":
			ds = tcp.NewClient(gwCfg.Downstream.Tcp.Address)
		case "rtu":
			ds = rtu.NewClient(gwCfg.Downstream.Serial)
		default:
			slog.Error("Unknown downstream type", "type", gwCfg.Downstream.Type, "gateway", gwCfg.Name)
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

		gw := gateway.NewGateway(gwCfg, upstreams, ds)
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
