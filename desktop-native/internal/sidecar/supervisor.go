// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package sidecar

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// errorTail is how many output lines explain a failed start or a crash.
const errorTail = 4

// Supervisor owns the gateway child process for the window: it starts,
// restarts and stops it, notices when it exits on its own, and serves the
// current child's data. It implements live.Runtime and live.Source.
type Supervisor struct {
	exe, config string
	notify      func() // called after every state change, e.g. to repaint
	out         Output

	mu     sync.Mutex
	state  live.State
	busy   bool // a start, restart or stop is in progress
	proc   *Process
	client *Client
}

// NewSupervisor prepares to run exe as the gateway on the config file. It
// starts in the stopped state; call Start.
func NewSupervisor(exe, config string, notify func()) *Supervisor {
	if notify == nil {
		notify = func() {}
	}
	return &Supervisor{exe: exe, config: config, notify: notify, state: live.State{Phase: live.Stopped}}
}

// Output is the gateway's recent output.
func (s *Supervisor) Output() *Output { return &s.out }

func (s *Supervisor) State() live.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Start launches the gateway in the background. A failed attempt changes
// the epoch; a successful first start keeps it.
func (s *Supervisor) Start() { s.begin(false) }

// Restart stops the running gateway, if any, and starts it again on the
// config file; either outcome changes the epoch. It does nothing while
// another start or restart is in progress.
func (s *Supervisor) Restart() { s.begin(true) }

func (s *Supervisor) begin(restart bool) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.state.Phase, s.state.Err = live.Starting, nil
	s.mu.Unlock()
	s.notify()
	go s.launch(restart)
}

func (s *Supervisor) launch(restart bool) {
	s.stopCurrent()
	p, err := Launch(s.exe, Args(s.config), &s.out)
	s.mu.Lock()
	s.busy = false
	if err != nil {
		s.state = live.State{Phase: live.Stopped, Err: s.explain(err), Epoch: s.state.Epoch + 1}
		s.mu.Unlock()
		s.notify()
		return
	}
	s.proc, s.client = p, NewClient(p.URL, p.Token)
	s.state.Phase = live.Running
	if restart {
		s.state.Epoch++
	}
	s.mu.Unlock()
	s.notify()
	go s.watch(p)
}

// watch reports an exit nobody asked for.
func (s *Supervisor) watch(p *Process) {
	<-p.Done()
	s.mu.Lock()
	if s.proc != p {
		s.mu.Unlock()
		return // stopped on purpose
	}
	s.proc = nil
	s.client.Close()
	s.state.Phase, s.state.Err = live.Stopped, s.explain(fmt.Errorf("网关意外退出（%v）", p.Err()))
	s.mu.Unlock()
	s.notify()
}

// stopCurrent stops the current child, if any, and waits for it.
func (s *Supervisor) stopCurrent() {
	s.mu.Lock()
	p, c := s.proc, s.client
	s.proc = nil
	s.mu.Unlock()
	if p != nil {
		c.Close()
		p.Stop()
	}
}

// Stop shuts the gateway down gracefully and waits for it; for app exit.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	s.busy = true // no restarts from here on
	s.mu.Unlock()
	s.stopCurrent()
	s.mu.Lock()
	s.state.Phase, s.state.Err = live.Stopped, errors.New("网关已停止")
	s.mu.Unlock()
}

// explain appends the gateway's last output lines to err.
func (s *Supervisor) explain(err error) error {
	tail := s.out.Tail(errorTail)
	if len(tail) == 0 {
		return err
	}
	return fmt.Errorf("%w\n%s", err, strings.Join(tail, "\n"))
}

// running returns the client of a running gateway, or nil.
func (s *Supervisor) running() *Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Phase != live.Running {
		return nil
	}
	return s.client
}

func (s *Supervisor) Since(seq uint64) []telemetry.Event {
	s.mu.Lock()
	c := s.client
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	return c.Since(seq)
}

func (s *Supervisor) Registers(sim string, t live.Table, start, count uint16) ([]uint16, bool) {
	if c := s.running(); c != nil {
		return c.Registers(sim, t, start, count)
	}
	return nil, false
}

func (s *Supervisor) Upstreams() []gateway.UpstreamStatus {
	if c := s.running(); c != nil {
		return c.Upstreams()
	}
	return nil
}
