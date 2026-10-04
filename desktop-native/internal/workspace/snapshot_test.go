package workspace

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
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

// Snapshots use fixture PDUs only; production never generates sample traffic.
func TestWorkspaceSnapshots(t *testing.T) {
	dir := os.Getenv("WORKSPACE_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("WORKSPACE_SNAPSHOT_DIR not set")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, size := range []image.Point{{1440, 900}, {1100, 680}} {
		win, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer win.Release()
			text := liveConfig("127.0.0.1:15020")
			r := telemetry.NewRecorder(100)
			base := time.Now()
			for i := 0; i < 20; i++ {
				r.Record(telemetry.Event{Time: base.Add(time.Duration(i-20) * time.Millisecond), Gateway: "demo", Downstream: "device", Source: "127.0.0.1:53124", SlaveID: 9, FunctionCode: 3, Address: 0, Quantity: 2, Request: []byte{3, 0, 0, 0, 2}, Response: []byte{3, 4, 0, 25, 0, 77}, Duration: 2 * time.Millisecond})
			}
			for _, state := range []string{"live", "menu", "config", "yaml", "startup-failed", "restart-confirm"} {
				info := Info{Config: parsed(t, text), Content: text, Running: true, Source: live.Local{Recorder: r}, Save: func(string) error { return nil }}
				if state == "startup-failed" {
					info.Running = false
					info.StartErr = errors.New("simulation model: failed to open persistence: permission denied")
				}
				u := New(info)
				u.world.Poll(base)
				u.view.link = Link{Gw: u.world.Gateways[0], Ds: u.world.Gateways[0].Downstreams[2]}
				if state == "live" {
					xs := u.world.Exchanges(nil)
					u.view.linked.selectRequest(xs[0])
					u.view.linked.packets[xs[0].Seq] = true
				}
				if state == "restart-confirm" {
					u.view.shell.confirming = true
				}
				if state == "menu" {
					u.view.shell.menu = 1
				}
				if state == "config" || state == "yaml" {
					u.view.shell.module = 1
					u.view.shell.cfg.sel = "gateways.0.downstreams.1"
				}
				if state == "yaml" {
					u.view.shell.cfg.raw = true
				}
				var ops op.Ops
				for i := 0; i < 2; i++ {
					ops.Reset()
					u.Layout(layout.Context{Ops: &ops, Now: base, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)})
					if err := win.Frame(&ops); err != nil {
						t.Fatal(err)
					}
				}
				img := image.NewRGBA(image.Rectangle{Max: size})
				if err := win.Screenshot(img); err != nil {
					t.Fatal(err)
				}
				name := state + "-" + fmtSize(size) + ".png"
				f, err := os.Create(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, img)
				closeErr := f.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
		}()
	}
}

func fmtSize(size image.Point) string {
	if size.X == 1100 {
		return "1100"
	}
	return "1440"
}
