// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package gateway

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ffutop/modbus-gateway/internal/modbus"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
	"github.com/ffutop/modbus-gateway/internal/transport"
)

type fakeDownstream struct {
	resp modbus.ProtocolDataUnit
	err  error
}

func (f *fakeDownstream) Send(context.Context, byte, modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	return f.resp, f.err
}
func (f *fakeDownstream) Connect(context.Context) error { return nil }
func (f *fakeDownstream) Close() error                  { return nil }

func TestHandleRequestRecordsRawPDUs(t *testing.T) {
	ok := &fakeDownstream{resp: modbus.ProtocolDataUnit{FunctionCode: 3, Data: []byte{2, 0x12, 0x34}}}
	failing := &fakeDownstream{err: errors.New("timeout")}
	rec := telemetry.NewRecorder(10)
	g := NewGateway("gw", nil, map[byte]transport.Downstream{1: ok, 2: failing}, nil)
	g.Telemetry = rec

	req := modbus.ProtocolDataUnit{FunctionCode: 3, Data: []byte{0, 0x10, 0, 1}}
	g.handleRequest(context.Background(), 1, req)
	g.handleRequest(context.Background(), 2, req)
	// The recorded bytes must not alias the caller's buffer.
	req.Data[1] = 0xFF

	events := rec.Since(0)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if want := []byte{3, 0, 0x10, 0, 1}; !bytes.Equal(events[0].Request, want) {
		t.Errorf("request = % x, want % x", events[0].Request, want)
	}
	if want := []byte{3, 2, 0x12, 0x34}; !bytes.Equal(events[0].Response, want) {
		t.Errorf("response = % x, want % x", events[0].Response, want)
	}
	if events[1].Err == nil || events[1].Response != nil {
		t.Errorf("failed request: err = %v, response = % x; want an error and no response", events[1].Err, events[1].Response)
	}
}

// fakeUpstream becomes ready, or fails with err, then serves until ctx ends.
type fakeUpstream struct {
	transport.Readiness
	err error
}

func (f *fakeUpstream) Start(ctx context.Context, _ transport.RequestHandler) error {
	if f.err != nil {
		return f.err
	}
	f.SetReady()
	<-ctx.Done()
	return nil
}
func (f *fakeUpstream) Close() error { return nil }

func waitStates(t *testing.T, g *Gateway, want ...UpstreamState) []UpstreamStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := g.UpstreamStatuses()
		match := len(got) == len(want)
		for i := 0; match && i < len(got); i++ {
			match = got[i].State == want[i]
		}
		if match {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("upstream states = %+v, want %v", got, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUpstreamStatusesFollowTheListenerLifecycle(t *testing.T) {
	g := NewGateway("gw", []transport.Upstream{&fakeUpstream{}, &fakeUpstream{err: errors.New("address in use")}}, nil, &fakeDownstream{})
	waitStates(t, g, UpstreamStarting, UpstreamStarting)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { g.Start(ctx); close(done) }()
	got := waitStates(t, g, UpstreamListening, UpstreamFailed)
	if got[1].Error != "address in use" || got[1].Gateway != "gw" || got[1].Index != 1 {
		t.Errorf("failed upstream = %+v", got[1])
	}

	cancel()
	<-done
	got = waitStates(t, g, UpstreamStopped, UpstreamFailed)
	if got[1].Error != "address in use" {
		t.Errorf("a failure must stay reported after shutdown: %+v", got[1])
	}
}
