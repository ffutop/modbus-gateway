package workspace

import (
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/configfile"
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
			for _, state := range []string{"live", "menu", "config", "yaml", "startup-failed", "restart-confirm", "config-gateway", "config-model", "config-delete-model", "config-empty", "config-overview", "config-search", "config-picker", "config-bulk", "config-advanced", "config-recovery", "config-wizard", "config-batch-delete", "config-create-gateway", "config-create-model", "config-create-reference", "config-conflict", "config-three-versions", "config-long-values"} {
				info := Info{Config: parsed(t, text), Content: text, Running: true, Source: live.Local{Recorder: r}, Save: func(string) error { return nil }, Rebase: func(string) error { return nil }}
				if state == "startup-failed" {
					info.Running = false
					info.StartErr = errors.New("simulation model: failed to open persistence: permission denied")
				}
				if state == "config-empty" {
					info.Content = "version: 1\nsimulations: []\ngateways: []\n"
					info.Config = parsed(t, info.Content)
				}
				if state == "config-recovery" {
					info.Draft = strings.Replace(text, "name: demo", "name: recovered", 1)
				}
				if state == "config-long-values" {
					longName := strings.Repeat("共享模拟模型", 12)
					info.Content = strings.ReplaceAll(text, "model", longName)
					info.Content = strings.Replace(info.Content, "name: demo", "name: "+strings.Repeat("生产车间网关", 12), 1)
					info.Content = strings.Replace(info.Content, "127.0.0.1:15021", "[2001:db8:1234:5678:90ab:cdef:1234:5678]:15021", 1)
					info.Config = parsed(t, info.Content)
				}
				info.Config.Path = "/workspace/modmux/site-production.yaml"
				if state == "config-long-values" {
					info.Config.Path = "/workspace/生产环境/" + strings.Repeat("现场配置文件", 10) + ".yaml"
				}
				u := New(info)
				u.world.Poll(base)
				if len(u.world.Gateways) > 0 {
					u.view.link = Link{Gw: u.world.Gateways[0], Ds: u.world.Gateways[0].Downstreams[2]}
				}
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
				if strings.HasPrefix(state, "config") || state == "yaml" {
					u.view.shell.module = 1
					u.view.shell.cfg.sel = "gateways.0.downstreams.1"
				}
				switch state {
				case "config-create-gateway":
					u.view.shell.cfg.openCreation("add-gateway", "")
				case "config-create-model":
					u.view.shell.cfg.openCreation("add-simulation", "")
				case "config-create-reference":
					u.view.shell.cfg.openCreation("new-model", "gateways.0.downstreams.0")
				case "config-conflict":
					e := u.view.shell.cfg
					e.conflict = &configfile.Conflict{Baseline: text, Disk: strings.Replace(text, "name: demo", "name: external", 1), Draft: strings.Replace(text, "name: demo", "name: draft", 1)}
					e.conflictView = "草稿"
					e.beginEdit()
					e.draft.Gateways[0].Name = "draft"
					e.eds["gateways.0.name"].SetText("draft")
					e.version++
					e.saveFailed = true
					e.conflictEditor.SetText(e.conflict.Draft)
				case "config-three-versions":
					e := u.view.shell.cfg
					e.beginEdit()
					e.draft.Gateways[0].Name = "saved-B"
					e.eds["gateways.0.name"].SetText("saved-B")
					if err := e.doSave(); err != nil {
						t.Fatal(err)
					}
					e.beginEdit()
					e.draft.Gateways[0].Name = "draft-C"
					e.eds["gateways.0.name"].SetText("draft-C")
					e.version++
					e.showDiff = true
					e.diffRunning = true
				case "config-long-values":
					u.view.shell.cfg.sel = "gateways.0.downstreams.2"
				case "config-gateway":
					u.view.shell.cfg.sel = "gateways.0"
				case "config-model", "config-delete-model":
					u.view.shell.cfg.sel = "simulations.0"
					if state == "config-delete-model" {
						u.view.shell.cfg.deletePath = "simulations.0"
						u.view.shell.cfg.replacements = map[string]string{}
					}
				case "config-overview":
					u.view.shell.cfg.sel = "group:网关"
				case "config-search":
					u.view.shell.cfg.sel = "group:下游"
					u.view.shell.cfg.wb.query.SetText("model")
				case "config-picker":
					u.view.shell.cfg.wb.picker = "gateways.0.downstreams.0.simulation.ref"
				case "config-bulk":
					u.view.shell.cfg.wb.bulk = newBulk("copy", []string{u.view.shell.cfg.node("gateways.0").id})
					u.view.shell.cfg.wb.bulk.independent.Value = true
					if err := u.view.shell.cfg.previewBulk(); err != nil {
						t.Fatal(err)
					}
				case "config-advanced":
					e := u.view.shell.cfg
					e.sel = "gateways.0.downstreams.2"
					e.draft.Gateways[0].Downstreams[2].Type = "rtu"
					e.transportDefaults(e.sel+".type", "rtu")
					e.draft.Gateways[0].Downstreams[2].Serial.Device = "/dev/ttyUSB0"
					e.resetControls()
					e.rebuild()
					e.wb.advanced[e.node(e.sel).id] = true
				case "config-wizard":
					if err := u.view.shell.cfg.startWizard(); err != nil {
						t.Fatal(err)
					}
				case "config-batch-delete":
					e := u.view.shell.cfg
					e.wb.bulk = newBulk("delete", []string{e.node("simulations.0").id, e.node("gateways.0").id})
					if err := e.previewBulk(); err != nil {
						t.Fatal(err)
					}
				case "config-empty":
					u.view.shell.cfg.sel = "group:网关"
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
