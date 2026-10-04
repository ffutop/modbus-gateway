package workspace

import (
	"image"
	"runtime"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// busyWorkspace returns a workspace with a full request history and a
// function that renders one frame 1/60 s later, after the requests that
// arrive at 1000/s in that time.
func busyWorkspace(tb testing.TB) (frame func()) {
	text := liveConfig("127.0.0.1:15020")
	c, err := config.ParseDraft([]byte(text))
	if err != nil {
		tb.Fatal(err)
	}
	rec := telemetry.NewRecorder(maxExchanges)
	u := New(Info{Config: c, Content: text, Running: true, Source: live.Local{Recorder: rec}})
	now := time.Now()
	read := func() {
		rec.Record(telemetry.Event{Time: now, Gateway: "demo", Downstream: "device", Source: "127.0.0.1:53124", SlaveID: 9, FunctionCode: 3, Address: 0, Quantity: 2, Request: []byte{3, 0, 0, 0, 2}, Response: []byte{3, 4, 0, 25, 0, 77}, Duration: 2 * time.Millisecond})
	}
	for i := 0; i < maxExchanges; i++ {
		read()
	}
	u.view.link = Link{Gw: u.world.Gateways[0], Ds: u.world.Gateways[0].Downstreams[2]}
	var ops op.Ops
	return func() {
		for i := 0; i < 17; i++ {
			read()
		}
		now = now.Add(time.Second / 60)
		ops.Reset()
		u.Layout(layout.Context{Ops: &ops, Now: now, Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}, Constraints: layout.Exact(image.Pt(2880, 1800))})
	}
}

func BenchmarkWorkspaceFrame(b *testing.B) {
	frame := busyWorkspace(b)
	for i := 0; i < 30; i++ {
		frame()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frame()
	}
}

// frameAlloc is the average heap allocation of one busy frame.
func frameAlloc(frame func(), n int) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < n; i++ {
		frame()
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / uint64(n)
}

// frameBudget bounds a busy frame's allocations: the view projects the
// history in place instead of copying it (10 MB per frame before).
const frameBudget = 200 << 10

func TestBusyFrameStaysWithinAllocationBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates on its own")
	}
	frame := busyWorkspace(t)
	for i := 0; i < 60; i++ {
		frame() // fonts, shaping caches, widget state
	}
	if got := frameAlloc(frame, 120); got > frameBudget {
		t.Fatalf("a busy frame allocates %d KB, budget %d KB", got>>10, frameBudget>>10)
	}
}

func TestIdleViewRefreshesSlowly(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	rec := telemetry.NewRecorder(10)
	w := newLiveWorld(parsed(t, text), live.Local{Recorder: rec})
	now := time.Now()
	w.Poll(now)
	if w.active(now) {
		t.Fatal("active without traffic")
	}
	rec.Record(telemetry.Event{Gateway: "demo", FunctionCode: 3})
	w.Poll(now.Add(pollInterval))
	if !w.active(now.Add(pollInterval)) {
		t.Fatal("not active right after traffic")
	}
	if w.active(now.Add(3 * time.Second)) {
		t.Fatal("still active long after traffic stopped")
	}
}
