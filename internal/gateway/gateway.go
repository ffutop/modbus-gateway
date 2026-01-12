// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package gateway

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/transport"
)

// Gateway represents a single gateway instance.
// It bridges multiple Upstreams (Masters) to a single Downstream (Slave).
type Gateway struct {
	Name       string
	Upstreams  []transport.Upstream
	Downstream transport.Downstream

	// Use direct dispatch with Mutex for simplicity and low latency.
	// Queuing could be added later if decoupling is needed.
	mu sync.Mutex // Mutex enables safe access to the Downstream serial port
}

// NewGateway creates a new Gateway instance
func NewGateway(cfg config.GatewayConfig, upstreams []transport.Upstream, downstream transport.Downstream) *Gateway {
	return &Gateway{
		Name:       cfg.Name,
		Upstreams:  upstreams,
		Downstream: downstream,
	}
}

// Start starts all upstream servers and the downstream connection
func (g *Gateway) Start(ctx context.Context) error {
	// Connect Downstream
	if err := g.Downstream.Connect(ctx); err != nil {
		slog.Error("Failed to connect downstream", "gateway", g.Name, "err", err)
		// We might continue even if downstream fails initially, it might recover
	}

	// Start Upstreams
	var wg sync.WaitGroup
	for i, us := range g.Upstreams {
		wg.Add(1)
		go func(ups transport.Upstream, idx int) {
			defer wg.Done()
			slog.Info("Starting upstream", "gateway", g.Name, "index", idx)
			if err := ups.Start(ctx, g.handleRequest); err != nil {
				slog.Error("Upstream stopped with error", "gateway", g.Name, "index", idx, "err", err)
			}
		}(us, i)
	}

	<-ctx.Done()

	// Graceful shutdown
	for _, us := range g.Upstreams {
		us.Close()
	}
	g.Downstream.Close()

	wg.Wait()
	return nil
}

// handleRequest is the central dispatch function
func (g *Gateway) handleRequest(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	// Serialize access to the downstream slave
	g.mu.Lock()
	defer g.mu.Unlock()

	// Forward to Downstream
	// Note: We might want to add a timeout here if the upstream doesn't provide one via context
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second) // Safety timeout
	defer cancel()

	respPdu, err := g.Downstream.Send(ctx, slaveID, pdu)
	if err != nil {
		slog.Error("Downstream request failed", "gateway", g.Name, "slaveID", slaveID, "func", pdu.FunctionCode, "err", err)
		return modbus.ProtocolDataUnit{}, err
	}

	return respPdu, nil
}
