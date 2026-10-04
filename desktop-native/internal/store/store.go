// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package store keeps the desktop app's request history: a bounded copy of
// the telemetry events, pulled from the recorder in batches, plus the
// latest per-gateway metrics.
package store

import (
	"sync"
	"time"

	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// Filter selects which events the request list shows.
type Filter struct {
	Gateway    string // empty = every gateway
	ErrorsOnly bool
}

func (f Filter) match(e *telemetry.Event) bool {
	return (f.Gateway == "" || e.Gateway == f.Gateway) && (!f.ErrorsOnly || e.Err != nil)
}

// Rate is a gateway's request rate over the last poll interval.
type Rate struct {
	Requests float64 // per second
	Errors   float64 // per second
}

// Store is safe for concurrent use: one goroutine polls, the UI reads.
type Store struct {
	capacity int

	mu      sync.RWMutex
	ring    []telemetry.Event
	total   int // events ever appended; event i (absolute) is ring[i%capacity]
	cursor  uint64
	missed  uint64 // events overwritten in the recorder before we read them
	filter  Filter
	visible []int // absolute indices of events matching filter, oldest first

	metrics  []telemetry.GatewayMetrics
	rates    map[string]Rate
	lastPoll time.Time
}

// New returns a Store keeping the most recent capacity events.
func New(capacity int) *Store {
	return &Store{capacity: max(capacity, 1), rates: map[string]Rate{}}
}

// Poll pulls new events and metrics from rec and reports whether anything
// changed.
func (s *Store) Poll(rec *telemetry.Recorder, now time.Time) bool {
	s.mu.RLock()
	cursor := s.cursor
	s.mu.RUnlock()
	events := rec.Since(cursor)
	metrics := rec.Metrics()

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range events {
		s.append(events[i])
	}
	return s.updateRates(metrics, now) || len(events) > 0
}

func (s *Store) append(e telemetry.Event) {
	if s.cursor > 0 && e.Seq > s.cursor+1 {
		s.missed += e.Seq - s.cursor - 1
	}
	s.cursor = e.Seq
	if s.ring == nil {
		s.ring = make([]telemetry.Event, s.capacity)
	}
	s.ring[s.total%s.capacity] = e
	if s.filter.match(&e) {
		s.visible = append(s.visible, s.total)
	}
	s.total++
	s.trim()
}

// trim drops visible indices whose events were overwritten.
func (s *Store) trim() {
	oldest := s.total - s.capacity
	i := 0
	for i < len(s.visible) && s.visible[i] < oldest {
		i++
	}
	if i > 0 {
		s.visible = append(s.visible[:0], s.visible[i:]...)
	}
}

// updateRates reports whether any rate changed.
func (s *Store) updateRates(metrics []telemetry.GatewayMetrics, now time.Time) bool {
	changed := len(metrics) != len(s.metrics)
	if !s.lastPoll.IsZero() {
		if dt := now.Sub(s.lastPoll).Seconds(); dt > 0 {
			prev := make(map[string]telemetry.Counts, len(s.metrics))
			for _, m := range s.metrics {
				prev[m.Name] = m.Counts
			}
			for _, m := range metrics {
				p := prev[m.Name]
				r := Rate{
					Requests: float64(m.Requests-p.Requests) / dt,
					Errors:   float64(m.Errors-p.Errors) / dt,
				}
				changed = changed || r != s.rates[m.Name]
				s.rates[m.Name] = r
			}
		}
	}
	s.metrics = metrics
	s.lastPoll = now
	return changed
}

// SetFilter changes which events Len and At expose.
func (s *Store) SetFilter(f Filter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f == s.filter {
		return
	}
	s.filter = f
	s.visible = s.visible[:0]
	for abs := max(s.total-s.capacity, 0); abs < s.total; abs++ {
		if f.match(&s.ring[abs%s.capacity]) {
			s.visible = append(s.visible, abs)
		}
	}
}

// Filter returns the current filter.
func (s *Store) Filter() Filter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filter
}

// Clear forgets every stored event; metrics are kept.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ring = nil
	s.total = 0
	s.visible = s.visible[:0]
	s.missed = 0
}

// Snapshot is a read-only view of the store, valid until Release.
type Snapshot struct {
	s *Store
}

// Read locks the store for reading; call Release when done. Holding it
// across a frame keeps indices stable while the list is laid out.
func (s *Store) Read() Snapshot {
	s.mu.RLock()
	return Snapshot{s}
}

// Release ends the snapshot.
func (v Snapshot) Release() { v.s.mu.RUnlock() }

// Len is the number of events matching the filter.
func (v Snapshot) Len() int { return len(v.s.visible) }

// At returns the i-th matching event, oldest first.
func (v Snapshot) At(i int) *telemetry.Event {
	return &v.s.ring[v.s.visible[i]%v.s.capacity]
}

// IndexOf returns the position of the event with the given Seq, or -1.
func (v Snapshot) IndexOf(seq uint64) int {
	lo, hi := 0, len(v.s.visible)
	for lo < hi {
		mid := (lo + hi) / 2
		if v.At(mid).Seq < seq {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(v.s.visible) && v.At(lo).Seq == seq {
		return lo
	}
	return -1
}

// Missed is the number of events the recorder overwrote before Poll saw
// them.
func (v Snapshot) Missed() uint64 { return v.s.missed }

// Metrics returns the latest per-gateway metrics, sorted by name.
func (v Snapshot) Metrics() []telemetry.GatewayMetrics { return v.s.metrics }

// Rate returns a gateway's recent request rate.
func (v Snapshot) Rate(gateway string) Rate { return v.s.rates[gateway] }
