// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package gateway

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ffutop/modbus-gateway/internal/routing"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
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

	// Telemetry, when set, receives one event per handled request.
	Telemetry *telemetry.Recorder
	// DownstreamNames labels downstreams in telemetry events.
	DownstreamNames map[transport.Downstream]string

	statusMu  sync.Mutex
	upstreams []UpstreamStatus // nil until Start
}

// UpstreamState is where an upstream is in its lifecycle. Failed and stopped
// are final: an upstream is not restarted.
type UpstreamState string

const (
	UpstreamStarting  UpstreamState = "starting"
	UpstreamListening UpstreamState = "listening"
	UpstreamFailed    UpstreamState = "failed"
	UpstreamStopped   UpstreamState = "stopped"
)

// UpstreamStatus reports one upstream, identified by its index in the
// gateway's configured upstreams.
type UpstreamStatus struct {
	Gateway string        `json:"gateway"`
	Index   int           `json:"index"`
	State   UpstreamState `json:"state"`
	Error   string        `json:"error,omitempty"`
}

// UpstreamStatuses returns every upstream's current state.
func (g *Gateway) UpstreamStatuses() []UpstreamStatus {
	g.statusMu.Lock()
	defer g.statusMu.Unlock()
	out := make([]UpstreamStatus, len(g.Upstreams))
	for i := range out {
		out[i] = UpstreamStatus{Gateway: g.Name, Index: i, State: UpstreamStarting}
		if i < len(g.upstreams) {
			out[i] = g.upstreams[i]
		}
	}
	return out
}

func (g *Gateway) setUpstream(idx int, state UpstreamState, err error) {
	g.statusMu.Lock()
	defer g.statusMu.Unlock()
	s := &g.upstreams[idx]
	if s.State == UpstreamFailed || s.State == UpstreamStopped {
		return
	}
	s.State, s.Error = state, ""
	if err != nil {
		s.Error = err.Error()
	}
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
//
// Deprecated: use internal/routing.ParseSlaveIDs directly. Kept as a thin
// delegate for existing call sites.
func ParseSlaveIDs(input string) ([]byte, error) {
	return routing.ParseSlaveIDs(input)
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
	g.statusMu.Lock()
	g.upstreams = make([]UpstreamStatus, len(g.Upstreams))
	for i := range g.upstreams {
		g.upstreams[i] = UpstreamStatus{Gateway: g.Name, Index: i, State: UpstreamStarting}
	}
	g.statusMu.Unlock()
	var wg sync.WaitGroup
	for i, us := range g.Upstreams {
		wg.Add(1)
		go func(ups transport.Upstream, idx int) {
			defer wg.Done()
			returned := make(chan struct{})
			if r, ok := ups.(transport.ReadyReporter); ok {
				go func() {
					select {
					case <-r.Ready():
						g.setUpstream(idx, UpstreamListening, nil)
					case <-returned:
					}
				}()
			}
			slog.Info("Starting upstream", "gateway", g.Name, "index", idx)
			err := ups.Start(ctx, g.handleRequest)
			close(returned)
			if err != nil {
				slog.Error("Upstream stopped with error", "gateway", g.Name, "index", idx, "err", err)
				g.setUpstream(idx, UpstreamFailed, err)
			} else {
				g.setUpstream(idx, UpstreamStopped, nil)
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
	if g.Telemetry == nil {
		resp, _, err := g.forward(ctx, slaveID, pdu)
		return resp, err
	}
	start := time.Now()
	resp, target, err := g.forward(ctx, slaveID, pdu)
	addr, qty := requestRange(pdu)
	g.Telemetry.Record(telemetry.Event{
		Time:         start,
		Gateway:      g.Name,
		Downstream:   g.DownstreamNames[target],
		Source:       transport.SourceAddr(ctx),
		SlaveID:      slaveID,
		FunctionCode: pdu.FunctionCode,
		Address:      addr,
		Quantity:     qty,
		Duration:     time.Since(start),
		Err:          err,
		Request:      pduBytes(pdu),
		Response:     responseBytes(resp, err),
	})
	return resp, err
}

// pduBytes copies pdu into its wire form, function code first.
func pduBytes(pdu modbus.ProtocolDataUnit) []byte {
	b := make([]byte, 0, 1+len(pdu.Data))
	return append(append(b, pdu.FunctionCode), pdu.Data...)
}

// responseBytes is the PDU returned by the downstream, or nil when the
// request failed; the upstream server then answers with its own exception.
func responseBytes(resp modbus.ProtocolDataUnit, err error) []byte {
	if err != nil {
		return nil
	}
	return pduBytes(resp)
}

// requestRange extracts the starting address and quantity of the standard
// read/write functions; other functions report zero.
func requestRange(pdu modbus.ProtocolDataUnit) (address, quantity uint16) {
	if len(pdu.Data) < 4 {
		return 0, 0
	}
	address = binary.BigEndian.Uint16(pdu.Data[0:2])
	switch pdu.FunctionCode {
	case 1, 2, 3, 4, 15, 16:
		return address, binary.BigEndian.Uint16(pdu.Data[2:4])
	case 5, 6:
		return address, 1
	}
	return 0, 0
}

// forward routes pdu to its downstream; target is nil when no route matched.
func (g *Gateway) forward(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, transport.Downstream, error) {
	// Route Lookup
	var target transport.Downstream
	if ds, ok := g.Routes[slaveID]; ok {
		target = ds
	} else if g.DefaultRoute != nil {
		target = g.DefaultRoute
	} else {
		// No route found
		slog.Warn("No route found for slave ID", "gateway", g.Name, "slaveID", slaveID)
		return modbus.ProtocolDataUnit{}, nil, fmt.Errorf("gateway path unavailable")
	}

	// Forward to Downstream
	// Note: We might want to add a timeout here if the upstream doesn't provide one via context
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second) // Safety timeout
	defer cancel()

	respPdu, err := target.Send(ctx, slaveID, pdu)
	if err != nil {
		slog.Error("Downstream request failed", "gateway", g.Name, "slaveID", slaveID, "func", pdu.FunctionCode, "err", err)
		return modbus.ProtocolDataUnit{}, target, err
	}

	return respPdu, target, nil
}
