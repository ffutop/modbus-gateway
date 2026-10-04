// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package live defines what the workspace reads from the running gateway: its
// data (Source) and its process state (Runtime). The app runs the gateway as
// a sidecar child process (package sidecar); Local reads an in-process
// runtime and only serves tests and development.
package live

import (
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// Table names a simulation table the way the management API does.
type Table string

const (
	Holding  Table = "holding_registers"
	Input    Table = "input_registers"
	Coils    Table = "coils"
	Discrete Table = "discrete_inputs"
)

// Source is the running gateway's data. The window goroutine calls it every
// frame, so implementations must answer from memory and never block on I/O.
type Source interface {
	// Since returns the requests with Seq greater than seq, in Seq order. A
	// gap in Seq means requests were dropped before the caller read them.
	Since(seq uint64) []telemetry.Event
	// Registers returns the window [start, start+count) of a simulation
	// table as last read; ok is false when the model is unavailable or not
	// read yet. Asking for a window is what keeps it refreshed.
	Registers(sim string, t Table, start, count uint16) (values []uint16, ok bool)
	// Upstreams reports the listeners' states, or nil while unknown.
	Upstreams() []gateway.UpstreamStatus
}

// Phase is where the gateway process is in its lifecycle.
type Phase int

const (
	Starting Phase = iota // launching, or restarting
	Running               // forwarding; listeners report their own states
	Stopped               // not running; State.Err says why
)

// State is a snapshot of the gateway process.
type State struct {
	Phase Phase
	Err   error // set when Stopped
	// Epoch changes whenever the workspace must be rebuilt from the config
	// file: after a restart, or after a start attempt failed.
	Epoch int
}

// Runtime controls the gateway process behind the workspace.
type Runtime interface {
	State() State
	// Restart stops the gateway and starts it again on the saved config file.
	// It returns at once; State follows the progress.
	Restart()
}

// Fixed is a Runtime that never changes, for tests and in-process use.
type Fixed State

func (f Fixed) State() State { return State(f) }
func (Fixed) Restart()       {}
