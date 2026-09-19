// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package injector

import (
	"context"
	"testing"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/local-slave/persistence"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/modbus"
)

func TestClient_Send_MapsCoilsToDiscreteInputs(t *testing.T) {
	sim, err := simulation.Open("sim", persistence.NewMemoryStorage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := NewClient(sim, []config.MappingConfig{
		{
			Source: config.MappingSourceConfig{Table: "coils", StartAddress: 0, Count: 16},
			Target: config.MappingTargetConfig{Table: "discrete_inputs", StartAddress: 100},
		},
	})

	resp, err := c.Send(context.Background(), 247, modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeWriteSingleCoil,
		Data:         []byte{0x00, 0x00, 0xFF, 0x00},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FunctionCode&0x80 != 0 {
		t.Fatalf("expected success response, got %+v", resp)
	}

	data, err := sim.Model.ReadDiscreteInputs(100, 1)
	if err != nil || data[0] != 1 {
		t.Errorf("expected target discrete input 100 to be set, got %v err=%v", data, err)
	}
}

func TestClient_Send_IllegalFunction(t *testing.T) {
	sim, err := simulation.Open("sim", persistence.NewMemoryStorage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := NewClient(sim, nil)

	resp, err := c.Send(context.Background(), 247, modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeReadCoils,
		Data:         []byte{0x00, 0x00, 0x00, 0x01},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data[0] != modbus.ExceptionCodeIllegalFunction {
		t.Errorf("expected IllegalFunction exception, got %+v", resp)
	}
}
