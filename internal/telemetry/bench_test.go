// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package telemetry

import (
	"fmt"
	"testing"
	"time"
)

// fullRecorder models a mid-size site: 4 gateways x 8 downstreams with every
// recent-events buffer full.
func fullRecorder() *Recorder {
	r := NewRecorder(1000)
	for i := 0; i < 4*1000; i++ {
		r.Record(Event{Gateway: fmt.Sprintf("gw%d", i%4), Downstream: fmt.Sprintf("ds%d", i%8), Duration: time.Duration(i%97) * time.Microsecond})
	}
	return r
}

// BenchmarkSinceNothingNew is what every open event stream does every 250ms
// when no request arrived since its last batch.
func BenchmarkSinceNothingNew(b *testing.B) {
	r := fullRecorder()
	cursor := r.Since(0)[len(r.Since(0))-1].Seq
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(r.Since(cursor)) != 0 {
			b.Fatal("unexpected events")
		}
	}
}

// BenchmarkMetrics is one metrics snapshot (the desktop monitor takes one per poll).
func BenchmarkMetrics(b *testing.B) {
	r := fullRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Metrics()
	}
}

// BenchmarkRecord is the cost added to every forwarded request.
func BenchmarkRecord(b *testing.B) {
	r := fullRecorder()
	e := Event{Gateway: "gw1", Downstream: "ds3", Duration: time.Millisecond}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.Record(e)
		}
	})
}
