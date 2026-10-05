// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package sidecar

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/config"
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

	mu                    sync.Mutex
	state                 live.State
	closed                bool
	usingRecovery         bool
	busy                  bool // a start, restart or stop is in progress
	runningText, lastGood string
	override              string
	proc                  *Process
	client                *Client
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
// the epoch; a healthy first start publishes its confirmed running baseline.
func (s *Supervisor) Start() { s.begin(false) }

// Restart stops the running gateway, if any, and starts it again on the
// config file; either outcome changes the epoch. It does nothing while
// another start or restart is in progress.
func (s *Supervisor) Restart() { s.begin(true) }

func (s *Supervisor) RestartWithConfig(text string) { s.beginConfig(true, text) }
func (s *Supervisor) RunningConfig() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Phase != live.Running {
		return ""
	}
	return s.runningText
}
func (s *Supervisor) LastGoodConfig() string { s.mu.Lock(); defer s.mu.Unlock(); return s.lastGood }
func (s *Supervisor) begin(restart bool)     { s.beginConfig(restart, "") }
func (s *Supervisor) beginConfig(restart bool, override string) {
	s.mu.Lock()
	if s.busy || s.closed {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.override = override
	s.state.Phase, s.state.Err = live.Starting, nil
	s.mu.Unlock()
	s.notify()
	go s.launch(restart)
}

func (s *Supervisor) launch(restart bool) {
	s.mu.Lock()
	override := s.override
	s.override = ""
	s.mu.Unlock()
	bytes, _ := os.ReadFile(s.config)
	var err error
	if override != "" {
		bytes = []byte(override)
		err = nil
	}
	s.stopCurrent()
	var p *Process
	if err == nil {
		path := s.config
		if override != "" {
			var file *os.File
			file, err = os.CreateTemp(filepath.Dir(s.config), ".modmux-running-*.yaml")
			if err == nil {
				path = file.Name()
				defer os.Remove(path)
				_, err = file.Write(bytes)
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
		if err == nil {
			p, err = Launch(s.exe, Args(path), &s.out)
		}
	}
	s.mu.Lock()
	if err != nil {
		s.busy = s.closed
		s.state = live.State{Phase: live.Stopped, Err: s.explain(err), Epoch: s.state.Epoch + 1}
		s.mu.Unlock()
		s.notify()
		return
	}
	s.proc, s.client = p, NewClient(p.URL, p.Token, s.notify)
	if s.closed {
		s.mu.Unlock()
		s.stopCurrent()
		return
	}
	s.state.Phase = live.Starting
	s.mu.Unlock()
	s.notify()
	go s.watch(p)
	go s.rememberHealthy(p, string(bytes), override != "")
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
	s.state.Epoch++
	s.busy = s.closed
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
	s.closed = true
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

// A ready management API alone does not establish that Modbus listeners bind.
func (s *Supervisor) rememberHealthy(p *Process, text string, recovered bool) {
	expected := 0
	if cfg, err := config.ParseDraft([]byte(text)); err == nil {
		for _, g := range cfg.Gateways {
			expected += len(g.Upstreams)
		}
	}
	deadline := time.NewTimer(readyTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.Done():
			return
		case <-deadline.C:
			s.failReadiness(p, fmt.Errorf("等待全部 Modbus 监听就绪超时"))
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.proc != p {
				s.mu.Unlock()
				return
			}
			client := s.client
			s.mu.Unlock()
			ups := client.Upstreams()
			if len(ups) == 0 {
				continue
			}
			healthy := len(ups) == expected
			for _, u := range ups {
				if u.State == gateway.UpstreamFailed {
					s.failReadiness(p, fmt.Errorf("Modbus 监听失败：%s / 上游 %d：%s", u.Gateway, u.Index+1, u.Error))
					return
				}
				if u.State != gateway.UpstreamListening {
					healthy = false
				}
			}
			if healthy {
				s.mu.Lock()
				if s.proc == p {
					s.lastGood, s.runningText = text, text
					s.usingRecovery = recovered
					s.state.Phase, s.state.Err = live.Running, nil
					s.state.Epoch++
					s.busy = false
				}
				s.mu.Unlock()
				s.notify()
				return
			}
		}
	}
}

func (s *Supervisor) UsingRecovery() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usingRecovery && s.state.Phase == live.Running
}

func (s *Supervisor) failReadiness(p *Process, err error) {
	s.mu.Lock()
	if s.proc != p || s.closed {
		s.mu.Unlock()
		return
	}
	client := s.client
	s.proc, s.client = nil, nil
	s.mu.Unlock()
	client.Close()
	p.Stop()
	s.mu.Lock()
	if !s.closed {
		s.state.Phase, s.state.Err = live.Stopped, s.explain(err)
		s.state.Epoch++
	}
	s.busy = s.closed
	s.mu.Unlock()
	s.notify()
}

func (s *Supervisor) ConnectionError() error {
	if client := s.running(); client != nil {
		return client.ConnectionError()
	}
	return nil
}
