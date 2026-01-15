// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/transport"
)

// Gateway represents a single gateway instance.
// It bridges multiple Upstreams (Masters) to multiple Downstreams (Slaves) using routing.
type Gateway struct {
	Name         string
	Upstreams    []transport.Upstream
	Routes       map[byte]transport.Downstream
	DefaultRoute transport.Downstream
}

// NewGateway creates a new Gateway instance
func NewGateway(name string, upstreams []transport.Upstream, routes map[byte]transport.Downstream, defaultRoute transport.Downstream) *Gateway {
	return &Gateway{
		Name:         name,
		Upstreams:    upstreams,
		Routes:       routes,
		DefaultRoute: defaultRoute,
	}
}

// ParseSlaveIDs parses a string of slave IDs (e.g. "1,2,5-10") into a slice of bytes.
func ParseSlaveIDs(input string) ([]byte, error) {
	var ids []byte
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			// Range
			ranges := strings.Split(part, "-")
			if len(ranges) != 2 {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			start, err := strconv.Atoi(strings.TrimSpace(ranges[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start of range: %w", err)
			}
			end, err := strconv.Atoi(strings.TrimSpace(ranges[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end of range: %w", err)
			}
			if start > end {
				return nil, fmt.Errorf("start of range %d is greater than end %d", start, end)
			}
			for i := start; i <= end; i++ {
				if i < 0 || i > 255 {
					return nil, fmt.Errorf("id out of range: %d", i)
				}
				ids = append(ids, byte(i))
			}
		} else {
			// Single
			id, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid id: %w", err)
			}
			if id < 0 || id > 255 {
				return nil, fmt.Errorf("id out of range: %d", id)
			}
			ids = append(ids, byte(id))
		}
	}
	return ids, nil
}

// Start starts all upstream servers and the downstream connection
func (g *Gateway) Start(ctx context.Context) error {
	// Connect Downstreams (Unique instances)
	uniqueDownstreams := make(map[transport.Downstream]struct{})
	for _, ds := range g.Routes {
		uniqueDownstreams[ds] = struct{}{}
	}
	if g.DefaultRoute != nil {
		uniqueDownstreams[g.DefaultRoute] = struct{}{}
	}

	for ds := range uniqueDownstreams {
		if err := ds.Connect(ctx); err != nil {
			slog.Error("Failed to connect downstream", "gateway", g.Name, "err", err)
			// We might continue even if downstream fails initially, it might recover
		}
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
	for ds := range uniqueDownstreams {
		ds.Close()
	}

	wg.Wait()
	return nil
}

// handleRequest is the central dispatch function
func (g *Gateway) handleRequest(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	// Route Lookup
	var target transport.Downstream
	if ds, ok := g.Routes[slaveID]; ok {
		target = ds
	} else if g.DefaultRoute != nil {
		target = g.DefaultRoute
	} else {
		// No route found
		slog.Warn("No route found for slave ID", "gateway", g.Name, "slaveID", slaveID)
		return modbus.ProtocolDataUnit{}, fmt.Errorf("gateway path unavailable")
	}

	// Forward to Downstream
	// Note: We might want to add a timeout here if the upstream doesn't provide one via context
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second) // Safety timeout
	defer cancel()

	respPdu, err := target.Send(ctx, slaveID, pdu)
	if err != nil {
		slog.Error("Downstream request failed", "gateway", g.Name, "slaveID", slaveID, "func", pdu.FunctionCode, "err", err)
		return modbus.ProtocolDataUnit{}, err
	}

	return respPdu, nil
}
