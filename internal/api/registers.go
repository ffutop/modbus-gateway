// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package api

import (
	"encoding/binary"
	"net/http"
	"strconv"

	"github.com/ffutop/modbus-gateway/internal/simulation"
)

// maxRegisterCount bounds one snapshot; a full screen of the register viewer
// is well below it.
const maxRegisterCount = 2048

// serveRegisters returns one table's values over [start, start+count) as a
// plain number array (0/1 for coils and discrete inputs).
func serveRegisters(w http.ResponseWriter, r *http.Request, sim *simulation.Simulation) {
	q := r.URL.Query()
	table := q.Get("table")
	start, err1 := strconv.ParseUint(q.Get("start"), 10, 16)
	count, err2 := strconv.ParseUint(q.Get("count"), 10, 16)
	if err1 != nil || err2 != nil || count == 0 || count > maxRegisterCount {
		writeError(w, http.StatusBadRequest, "start must be 0-65535 and count 1-%d", maxRegisterCount)
		return
	}
	addr, qty := uint16(start), uint16(count)

	var raw []byte
	var err error
	bits := table == "coils" || table == "discrete_inputs"
	switch table {
	case "coils":
		raw, err = sim.Model.ReadCoils(addr, qty)
	case "discrete_inputs":
		raw, err = sim.Model.ReadDiscreteInputs(addr, qty)
	case "holding_registers":
		raw, err = sim.Model.ReadHoldingRegisters(addr, qty)
	case "input_registers":
		raw, err = sim.Model.ReadInputRegisters(addr, qty)
	default:
		writeError(w, http.StatusBadRequest, "unknown table %q", table)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}

	values := make([]uint16, qty)
	for i := range values {
		if bits {
			values[i] = uint16(raw[i/8]>>(i%8)) & 1
		} else {
			values[i] = binary.BigEndian.Uint16(raw[i*2:])
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"table": table, "start": start, "values": values})
}
