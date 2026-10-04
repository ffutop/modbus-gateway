// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package store

import (
	"errors"
	"testing"
	"time"

	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

func record(rec *telemetry.Recorder, gateway string, fail bool) {
	e := telemetry.Event{Gateway: gateway, FunctionCode: 3}
	if fail {
		e.Err = errors.New("boom")
	}
	rec.Record(e)
}

func seqs(s *Store) []uint64 {
	v := s.Read()
	defer v.Release()
	out := make([]uint64, v.Len())
	for i := range out {
		out[i] = v.At(i).Seq
	}
	return out
}

func equal(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPollKeepsMostRecentEvents(t *testing.T) {
	rec := telemetry.NewRecorder(100)
	s := New(3)
	for i := 0; i < 5; i++ {
		record(rec, "a", false)
	}
	if !s.Poll(rec, time.Now()) {
		t.Fatal("Poll with new events should report a change")
	}
	if got := seqs(s); !equal(got, []uint64{3, 4, 5}) {
		t.Fatalf("seqs = %v, want [3 4 5]", got)
	}
	record(rec, "a", false)
	s.Poll(rec, time.Now())
	if got := seqs(s); !equal(got, []uint64{4, 5, 6}) {
		t.Fatalf("after one more: seqs = %v, want [4 5 6]", got)
	}
}

func TestFilterAppliesToStoredAndNewEvents(t *testing.T) {
	rec := telemetry.NewRecorder(100)
	s := New(10)
	record(rec, "a", false) // 1
	record(rec, "b", true)  // 2
	record(rec, "a", true)  // 3
	s.Poll(rec, time.Now())

	s.SetFilter(Filter{Gateway: "a"})
	if got := seqs(s); !equal(got, []uint64{1, 3}) {
		t.Fatalf("gateway a: %v, want [1 3]", got)
	}
	s.SetFilter(Filter{ErrorsOnly: true})
	if got := seqs(s); !equal(got, []uint64{2, 3}) {
		t.Fatalf("errors only: %v, want [2 3]", got)
	}
	record(rec, "a", false) // 4, filtered out
	record(rec, "b", true)  // 5
	s.Poll(rec, time.Now())
	if got := seqs(s); !equal(got, []uint64{2, 3, 5}) {
		t.Fatalf("errors only after poll: %v, want [2 3 5]", got)
	}
	v := s.Read()
	defer v.Release()
	if i := v.IndexOf(3); i != 1 {
		t.Errorf("IndexOf(3) = %d, want 1", i)
	}
	if i := v.IndexOf(4); i != -1 {
		t.Errorf("IndexOf(4) = %d, want -1 (filtered out)", i)
	}
}

func TestMissedCountsOverwrittenEvents(t *testing.T) {
	rec := telemetry.NewRecorder(2)
	s := New(10)
	record(rec, "a", false)
	s.Poll(rec, time.Now())
	for i := 0; i < 4; i++ { // seqs 2..5; the recorder keeps only 4 and 5
		record(rec, "a", false)
	}
	s.Poll(rec, time.Now())
	v := s.Read()
	defer v.Release()
	if v.Missed() != 2 {
		t.Errorf("Missed = %d, want 2", v.Missed())
	}
}

func TestRates(t *testing.T) {
	rec := telemetry.NewRecorder(100)
	s := New(10)
	t0 := time.Now()
	s.Poll(rec, t0)
	record(rec, "a", false)
	record(rec, "a", true)
	s.Poll(rec, t0.Add(500*time.Millisecond))
	v := s.Read()
	defer v.Release()
	if r := v.Rate("a"); r.Requests != 4 || r.Errors != 2 {
		t.Errorf("rate = %+v, want 4 req/s and 2 err/s", r)
	}
}
