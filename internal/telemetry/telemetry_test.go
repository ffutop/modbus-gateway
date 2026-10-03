// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package telemetry

import (
	"testing"
	"time"
)

func TestMetrics_LatencyPercentilesFromRecentEvents(t *testing.T) {
	r := NewRecorder(1000)
	for i := 1; i <= 100; i++ {
		r.Record(Event{Gateway: "gw", Downstream: "plc", Duration: time.Duration(i) * time.Millisecond})
	}
	r.Record(Event{Gateway: "gw", Downstream: "other", Duration: 500 * time.Millisecond})

	m := r.Metrics()
	if len(m) != 1 {
		t.Fatalf("metrics = %+v", m)
	}
	var plc Counts
	for _, d := range m[0].Downstreams {
		if d.Name == "plc" {
			plc = d
		}
	}
	if plc.P50Ms != 50 || plc.P99Ms != 99 {
		t.Errorf("plc p50/p99 = %v/%v ms, want 50/99", plc.P50Ms, plc.P99Ms)
	}
	if m[0].P99Ms != 100 {
		t.Errorf("gateway p99 = %v ms, want 100 (includes the 500ms outlier only at the very top)", m[0].P99Ms)
	}
}

func TestMetrics_PercentilesOnlyCoverTheLastCapacityEvents(t *testing.T) {
	r := NewRecorder(10)
	for i := 0; i < 10; i++ {
		r.Record(Event{Gateway: "gw", Duration: time.Second})
	}
	for i := 0; i < 10; i++ {
		r.Record(Event{Gateway: "gw", Duration: time.Millisecond})
	}

	g := r.Metrics()[0]
	if g.Requests != 20 {
		t.Errorf("requests = %d, want 20 (counts are cumulative)", g.Requests)
	}
	if g.P99Ms != 1 {
		t.Errorf("p99 = %v ms, want 1: the 1s events were overwritten", g.P99Ms)
	}
}
