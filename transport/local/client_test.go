// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package local

import (
	"context"
	"testing"

	"github.com/ffutop/modbus-gateway/internal/local-slave/persistence"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/modbus"
)

func TestClient_Send_DelegatesToSharedSimulation(t *testing.T) {
	sim, err := simulation.Open("sim", persistence.NewMemoryStorage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := NewClient(sim)

	resp, err := c.Send(context.Background(), 1, modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteSingleCoil,
		Data:         []byte{0x00, 0x00, 0xFF, 0x00},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FunctionCode&0x80 != 0 {
		t.Fatalf("expected success response, got %+v", resp)
	}
	if sim.Version() != 1 {
		t.Errorf("expected the shared simulation to observe the write, got version %d", sim.Version())
	}
}
