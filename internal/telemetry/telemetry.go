// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package telemetry records what happens on the forwarding path for the
// management API and the desktop app. It is in-memory debug data, cleared on
// restart; it is
// not an audit log.
package telemetry

import (
	"math"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Event is one request handled by a gateway.
type Event struct {
	Seq          uint64 // assigned by Record; increases across all gateways
	Time         time.Time
	Gateway      string
	Downstream   string // empty when no route matched
	Source       string // upstream connection address, if known
	SlaveID      byte
	FunctionCode byte
	Address      uint16 // first address of the request, if the function has one
	Quantity     uint16
	Duration     time.Duration
	Err          error
	// Request and Response are the raw PDUs, function code first. Response
	// is nil when Err is set.
	Request  []byte
	Response []byte
}

// Recorder accumulates Events. It is safe for concurrent use and never
// blocks the caller for longer than a map lookup. A nil *Recorder records
// nothing and reports no events, so readers need no nil checks.
type Recorder struct {
	capacity int
	seq      atomic.Uint64

	mu       sync.RWMutex
	gateways map[string]*gatewayStats
}

type counters struct {
	requests atomic.Uint64
	errors   atomic.Uint64
}

func (c *counters) add(err error) {
	c.requests.Add(1)
	if err != nil {
		c.errors.Add(1)
	}
}

type gatewayStats struct {
	counters
	mu          sync.RWMutex
	downstreams map[string]*counters

	ringMu sync.Mutex
	ring   []Event // most recent events, oldest overwritten first
	next   int
	full   bool
}

// NewRecorder returns a Recorder keeping up to capacity recent events per
// gateway.
func NewRecorder(capacity int) *Recorder {
	capacity = max(capacity, 1)
	return &Recorder{capacity: capacity, gateways: map[string]*gatewayStats{}}
}

// Record adds one event.
func (r *Recorder) Record(e Event) {
	e.Seq = r.seq.Add(1)
	g := r.gateway(e.Gateway)
	g.add(e.Err)
	if e.Downstream != "" {
		g.downstream(e.Downstream).add(e.Err)
	}
	g.ringMu.Lock()
	g.ring[g.next] = e
	g.next = (g.next + 1) % len(g.ring)
	if g.next == 0 {
		g.full = true
	}
	g.ringMu.Unlock()
}

// newerThan returns the buffered events with Seq > seq, oldest first. It
// walks back from the newest event and stops at the first older one, so a
// poll with nothing new copies nothing.
func (g *gatewayStats) newerThan(seq uint64) []Event {
	g.ringMu.Lock()
	defer g.ringMu.Unlock()
	n := g.next
	if g.full {
		n = len(g.ring)
	}
	var out []Event
	for k := 1; k <= n; k++ {
		e := g.ring[(g.next-k+len(g.ring))%len(g.ring)]
		if e.Seq <= seq {
			break
		}
		out = append(out, e)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// latencies returns the durations of the buffered events, sorted, for the
// gateway as a whole and per downstream, in one pass over the buffer.
func (g *gatewayStats) latencies() (all []time.Duration, byDownstream map[string][]time.Duration) {
	g.ringMu.Lock()
	n := g.next
	if g.full {
		n = len(g.ring)
	}
	all = make([]time.Duration, 0, n)
	byDownstream = make(map[string][]time.Duration)
	for _, e := range g.ring[:n] {
		all = append(all, e.Duration)
		if e.Downstream != "" {
			byDownstream[e.Downstream] = append(byDownstream[e.Downstream], e.Duration)
		}
	}
	g.ringMu.Unlock()

	sortDurations(all)
	for _, d := range byDownstream {
		sortDurations(d)
	}
	return all, byDownstream
}

func sortDurations(d []time.Duration) { slices.Sort(d) }

func (r *Recorder) gateway(name string) *gatewayStats {
	r.mu.RLock()
	g, ok := r.gateways[name]
	r.mu.RUnlock()
	if ok {
		return g
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok = r.gateways[name]; !ok {
		g = &gatewayStats{downstreams: map[string]*counters{}, ring: make([]Event, r.capacity)}
		r.gateways[name] = g
	}
	return g
}

func (g *gatewayStats) downstream(name string) *counters {
	g.mu.RLock()
	c, ok := g.downstreams[name]
	g.mu.RUnlock()
	if ok {
		return c
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok = g.downstreams[name]; !ok {
		c = &counters{}
		g.downstreams[name] = c
	}
	return c
}

// Since returns the buffered events with Seq greater than seq, across all
// gateways, in Seq order. Events already overwritten are skipped.
func (r *Recorder) Since(seq uint64) []Event {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Event
	for _, g := range r.gateways {
		out = append(out, g.newerThan(seq)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// Counts holds cumulative request/error counts, plus latency percentiles
// over the events still in the recent-events buffer. Clients derive request
// rates by diffing Requests between two snapshots.
type Counts struct {
	Name     string  `json:"name"`
	Requests uint64  `json:"requests"`
	Errors   uint64  `json:"errors"`
	P50Ms    float64 `json:"p50_ms"`
	P99Ms    float64 `json:"p99_ms"`
}

// GatewayMetrics is the snapshot for one gateway and its downstreams.
type GatewayMetrics struct {
	Counts
	Downstreams []Counts `json:"downstreams"`
}

// Metrics returns a snapshot of every gateway seen so far, sorted by name.
func (r *Recorder) Metrics() []GatewayMetrics {
	if r == nil {
		return []GatewayMetrics{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]GatewayMetrics, 0, len(r.gateways))
	for name, g := range r.gateways {
		all, byDownstream := g.latencies()
		gm := GatewayMetrics{Counts: g.snapshot(name, all), Downstreams: []Counts{}}
		g.mu.RLock()
		for dname, c := range g.downstreams {
			gm.Downstreams = append(gm.Downstreams, c.snapshot(dname, byDownstream[dname]))
		}
		g.mu.RUnlock()
		sort.Slice(gm.Downstreams, func(i, j int) bool { return gm.Downstreams[i].Name < gm.Downstreams[j].Name })
		out = append(out, gm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// snapshot reads c, with percentiles over the given sorted recent durations.
func (c *counters) snapshot(name string, durations []time.Duration) Counts {
	return Counts{
		Name:     name,
		Requests: c.requests.Load(),
		Errors:   c.errors.Load(),
		P50Ms:    percentileMs(durations, 0.50),
		P99Ms:    percentileMs(durations, 0.99),
	}
}

// percentileMs is the nearest-rank percentile of sorted durations, in ms.
func percentileMs(sorted []time.Duration, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	return float64(sorted[max(i, 0)]) / float64(time.Millisecond)
}
