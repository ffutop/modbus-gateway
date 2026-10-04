// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package live

import (
	"encoding/binary"

	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// Local reads a runtime in this process. Any field may be nil.
type Local struct {
	Recorder    *telemetry.Recorder
	Simulations map[string]*simulation.Simulation
	Statuses    func() []gateway.UpstreamStatus
}

func (l Local) Since(seq uint64) []telemetry.Event { return l.Recorder.Since(seq) }

func (l Local) Registers(sim string, t Table, start, count uint16) ([]uint16, bool) {
	s := l.Simulations[sim]
	if s == nil {
		return nil, false
	}
	var raw []byte
	var err error
	switch t {
	case Holding:
		raw, err = s.Model.ReadHoldingRegisters(start, count)
	case Input:
		raw, err = s.Model.ReadInputRegisters(start, count)
	case Coils:
		raw, err = s.Model.ReadCoils(start, count)
	case Discrete:
		raw, err = s.Model.ReadDiscreteInputs(start, count)
	default:
		return nil, false
	}
	if err != nil {
		return nil, false
	}
	return Decode(t, raw, int(count)), true
}

func (l Local) Upstreams() []gateway.UpstreamStatus {
	if l.Statuses == nil {
		return nil
	}
	return l.Statuses()
}

// Decode unpacks count values of table t from Modbus response bytes: bits
// for coils and discrete inputs, big-endian words for registers.
func Decode(t Table, raw []byte, count int) []uint16 {
	values := make([]uint16, count)
	for i := range values {
		if t == Coils || t == Discrete {
			values[i] = uint16(raw[i/8]>>uint(i%8)) & 1
		} else {
			values[i] = binary.BigEndian.Uint16(raw[i*2:])
		}
	}
	return values
}
