// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package transport

import "sync"

// ReadyReporter is implemented by upstreams that can tell when they start
// accepting requests: the listening socket is bound or the serial port open.
type ReadyReporter interface {
	Ready() <-chan struct{}
}

// Readiness implements ReadyReporter for an upstream that embeds it and calls
// SetReady once it accepts requests. The zero value is not ready.
type Readiness struct {
	mu    sync.Mutex
	ch    chan struct{}
	ready bool
}

func (r *Readiness) chanLocked() chan struct{} {
	if r.ch == nil {
		r.ch = make(chan struct{})
	}
	return r.ch
}

// Ready returns a channel closed once SetReady has been called.
func (r *Readiness) Ready() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.chanLocked()
}

// SetReady marks the upstream ready; later calls do nothing.
func (r *Readiness) SetReady() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ready {
		r.ready = true
		close(r.chanLocked())
	}
}
