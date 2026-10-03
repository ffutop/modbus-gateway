// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package telemetry

import (
	"fmt"
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

func TestSince_ReturnsNewerEventsAcrossGatewaysInOrder(t *testing.T) {
	r := NewRecorder(3)
	for i := 0; i < 5; i++ { // gw-a keeps only its last 3 (seq 5,7,9)
		r.Record(Event{Gateway: "gw-a", SlaveID: byte(i)})
		r.Record(Event{Gateway: "gw-b", SlaveID: byte(i)})
	}
	all := r.Since(0)
	var seqs []uint64
	for _, e := range all {
		seqs = append(seqs, e.Seq)
	}
	if fmt.Sprint(seqs) != "[5 6 7 8 9 10]" {
		t.Fatalf("Since(0) seqs = %v, want the 3 kept per gateway in order", seqs)
	}
	if got := r.Since(8); len(got) != 2 || got[0].Seq != 9 || got[1].Seq != 10 {
		t.Errorf("Since(8) = %+v, want seq 9 and 10", got)
	}
	if got := r.Since(10); len(got) != 0 {
		t.Errorf("Since(10) = %+v, want nothing", got)
	}
}
