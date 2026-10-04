// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package ui

import (
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/store"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// TestSnapshot renders the window with sample traffic into
// $UI_SNAPSHOT_DIR/main.png, for reviewing the layout without a display:
//
//	UI_SNAPSHOT_DIR=/tmp go test ./internal/ui -run Snapshot
func TestSnapshot(t *testing.T) {
	dir := os.Getenv("UI_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("UI_SNAPSHOT_DIR not set")
	}
	const scale = 2
	size := image.Pt(1280*scale, 800*scale)
	win, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("no headless GPU context: %v", err)
	}
	defer win.Release()

	rec := telemetry.NewRecorder(100)
	base := time.Date(2026, 10, 4, 9, 30, 0, 0, time.Local)
	samples := []telemetry.Event{
		{Gateway: "local-demo", Downstream: "slave-1", SlaveID: 1, FunctionCode: 3, Address: 0, Quantity: 2,
			Request: []byte{3, 0, 0, 0, 2}, Response: []byte{3, 4, 0, 10, 0x12, 0x34}},
		{Gateway: "local-demo", Downstream: "slave-1", SlaveID: 1, FunctionCode: 6, Address: 1, Quantity: 1,
			Request: []byte{6, 0, 1, 4, 0xD2}, Response: []byte{6, 0, 1, 4, 0xD2}},
		{Gateway: "local-demo", SlaveID: 4, FunctionCode: 3, Quantity: 10,
			Request: []byte{3, 0, 0, 0, 10}, Err: errors.New("gateway path unavailable")},
		{Gateway: "plc-line-2", Downstream: "plc", SlaveID: 1, FunctionCode: 16, Address: 2, Quantity: 3,
			Request: []byte{16, 0, 2, 0, 3, 6, 0x12, 0x34, 0xAB, 0xCD, 0, 7}, Response: []byte{16, 0, 2, 0, 3}},
	}
	for i := 0; i < 40; i++ {
		e := samples[i%len(samples)]
		e.Time = base.Add(time.Duration(i) * 37 * time.Millisecond)
		e.Source = "127.0.0.1:52114"
		e.Duration = time.Duration(300+i*41) * time.Microsecond
		rec.Record(e)
	}
	st := store.New(1000)
	st.Poll(rec, base)
	st.Poll(rec, base.Add(time.Second))

	u := New(st, Info{ConfigPath: "/etc/modmux/demo.yaml", Gateways: []string{"local-demo", "plc-line-2"}})
	snap := st.Read()
	u.table.sel, u.table.hasSel = *snap.At(snap.Len() - 1), true
	snap.Release()
	u.inspector.load(&u.table.sel)
	u.inspector.selected = fieldRef{0, 5} // first written register

	var ops op.Ops
	for i := 0; i < 2; i++ { // the second frame sees the first frame's layout state
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Now:         base,
			Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
			Constraints: layout.Exact(size),
		}
		u.Layout(gtx)
		if err := win.Frame(&ops); err != nil {
			t.Fatal(err)
		}
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if err := win.Screenshot(img); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "main.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
