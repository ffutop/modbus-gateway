// PROTOTYPE — throwaway. -snapshot renders the variants offscreen to PNGs,
// to review layouts without a display.

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func snapshot(dir string) error { return snapshotSize(dir, 1440, 900) }

func snapshotSize(dir string, width, height int) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	const scale = 2
	size := image.Pt(width*scale, height*scale)
	win, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		return err
	}
	defer win.Release()

	world := newWorld()
	now := time.Now()
	for i := 0; i < 600; i++ {
		t := now.Add(time.Duration(i-600) * 25 * time.Millisecond)
		world.drift(t, float64(i)/40)
		world.request(t)
	}
	for k, c := range world.counts {
		world.rates[k] = float64(c[0]) / 15
	}
	th := NewTheme()
	cfg := newConfigEditor(th, world)
	c1 := newVariantC1(th, world, cfg)
	variants := []variant{c1}

	pick := func(l Link, keep func(*Exchange) bool) (Exchange, bool) {
		xs := world.Exchanges(func(x *Exchange) bool { return l.Match(x) && keep(x) })
		if len(xs) < 3 {
			return Exchange{}, false
		}
		return xs[len(xs)-3], true
	}
	scrollTo := func(lv *linkedView, l Link, seq uint64) {
		for i, x := range world.Exchanges(l.Match) {
			if x.Seq == seq {
				lv.traffic.Position.First = max(i-2, 0)
			}
		}
	}
	render := func(name string) error {
		sw := &switcher{th: th, variants: variants}
		var ops op.Ops
		for f := 0; f < 2; f++ {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Now: now, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			sw.Layout(gtx)
			if err := win.Frame(&ops); err != nil {
				return err
			}
		}
		img := image.NewRGBA(image.Rectangle{Max: size})
		if err := win.Screenshot(img); err != nil {
			return err
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			return err
		}
		defer f.Close()
		return png.Encode(f, img)
	}

	// A real TCP device, narrowed by the topology: one master and one device.
	g2 := world.Gateways[1]
	c1.link = Link{Gw: g2, Master: g2.Masters[0], Ds: g2.Downstreams[1], SlaveFilter: 10}
	c1.linked.slave = 9
	if x, ok := pick(c1.link, func(x *Exchange) bool { return x.FunctionCode == 3 }); ok {
		c1.linked.selectRequest(x)
		scrollTo(c1.linked, c1.link, x.Seq)
	}
	if err := render("link-device"); err != nil {
		return err
	}
	c1.link.SlaveFilter = 11
	c1.linked.reset()
	c1.linked.slave = 10
	if x, ok := pick(c1.link, func(x *Exchange) bool { return x.FunctionCode == 3 }); ok {
		c1.linked.selectRequest(x)
		scrollTo(c1.linked, c1.link, x.Seq)
	}
	if err := render("link-device-id10"); err != nil {
		return err
	}
	// A model-backed downstream on the first gateway.
	g1 := world.Gateways[0]
	c1.link = Link{Gw: g1, Ds: g1.Downstreams[0]}
	c1.linked.reset()
	if x, ok := pick(c1.link, func(x *Exchange) bool { return x.FunctionCode == 16 }); ok {
		c1.linked.selectRequest(x)
		scrollTo(c1.linked, c1.link, x.Seq)
	}
	if err := render("link-model"); err != nil {
		return err
	}
	c1.link = Link{Gw: g1, Ds: g1.Downstreams[1]}
	c1.linked.reset()
	c1.linked.table = input
	c1.linked.regList.Position.First = 58
	if x, ok := pick(c1.link, func(x *Exchange) bool { return x.FunctionCode == 16 }); ok {
		c1.linked.selectRequest(x)
		scrollTo(c1.linked, c1.link, x.Seq)
		c1.linked.packets[x.Seq] = true
	}
	if err := render("link-injector"); err != nil {
		return err
	}
	c1.link = Link{Gw: g1, Ds: g1.Downstreams[2]}
	c1.linked.reset()
	c1.linked.table = holding
	c1.linked.regList.Position.First = 0
	if x, ok := pick(c1.link, func(x *Exchange) bool { return x.Err != nil }); ok {
		c1.linked.selectRequest(x)
		scrollTo(c1.linked, c1.link, x.Seq)
		c1.linked.packets[x.Seq] = true
	}
	if err := render("link-timeout"); err != nil {
		return err
	}
	// Visual configuration with edits, a real config Problem and the change list.
	c1.shell.module = 1
	cfg.eds["gateways.0.downstreams.0.slave_ids"].SetText("2")
	cfg.eds["gateways.0.downstreams.0.name"].SetText("锅炉 PLC（主）")
	cfg.draft.Log.Level = "debug"
	cfg.showDiff = true
	if err := render("config-visual"); err != nil {
		return err
	}
	// Raw mode: the same draft as YAML, with a comment and a broken edit.
	cfg = newConfigEditor(th, world)
	c1.shell.cfg = cfg
	cfg.sel = "gateways.0.downstreams.1"
	if err := render("config-mappings"); err != nil {
		return err
	}
	cfg.mapAdd.get("gateways.0.downstreams.1").Click()
	if err := render("config-mapping-conflict"); err != nil {
		return err
	}
	if len(cfg.draft.Problems()) == 0 {
		return fmt.Errorf("mapping overlap was not diagnosed")
	}
	cfg.mapRemove.get("gateways.0.downstreams.1.1").Click()
	if err := render("config-mapping-fixed"); err != nil {
		return err
	}
	if len(cfg.draft.Problems()) != 0 {
		return fmt.Errorf("mapping removal left a conflict")
	}
	cfg.eds["gateways.0.downstreams.0.name"].SetText("锅炉 PLC（主）")
	cfg.save.Click()
	cfg.showDiff = true
	if err := render("config-pending"); err != nil {
		return err
	}
	if cfg.savedYAML == cfg.runningYAML || len(cfg.pendingChanges()) == 0 {
		return fmt.Errorf("save lost pending application")
	}
	cfg.simulateFailure = true
	cfg.apply.Click()
	if err := render("config-confirm"); err != nil {
		return err
	}
	cfg.confirm.Click()
	if err := render("config-apply-failed"); err != nil {
		return err
	}
	if !cfg.applyFailed || cfg.savedYAML == cfg.runningYAML {
		return fmt.Errorf("failed application did not preserve running configuration")
	}
	cfg.apply.Click()
	if err := render("config-retry-confirm"); err != nil {
		return err
	}
	cfg.confirm.Click()
	if err := render("config-applying"); err != nil {
		return err
	}
	now = cfg.applyUntil.Add(time.Millisecond)
	if err := render("config-applied"); err != nil {
		return err
	}
	if cfg.applyFailed || cfg.savedYAML != cfg.runningYAML {
		return fmt.Errorf("retry did not apply saved configuration")
	}
	cfg = newConfigEditor(th, world)
	c1.shell.cfg = cfg
	cfg.raw, cfg.showDiff = true, false
	text := "# 产线 1 与仓储两个网关\n" + toYAML(cfg.draft)
	cfg.rawEd.SetText(strings.Replace(text, "baud_rate: 19200", "baud_rate: fast", 1))
	cfg.reparse()
	if err := render("config-raw"); err != nil {
		return err
	}
	c1.shell.menus[2].Click()
	if err := render("desktop-menu"); err != nil {
		return err
	}
	if c1.shell.menu != 3 {
		return fmt.Errorf("view menu did not open")
	}
	c1.shell.commands[5].Click()
	if err := render("desktop-workspace"); err != nil {
		return err
	}
	if c1.shell.module != 0 || c1.shell.menu != 0 {
		return fmt.Errorf("view menu did not switch workspace and close")
	}
	c1.shell.menus[0].Click()
	if err := render("desktop-file-menu"); err != nil {
		return err
	}
	c1.shell.commands[1].Click()
	if err := render("desktop-menu-config"); err != nil {
		return err
	}
	if c1.shell.module != 1 || !cfg.raw {
		return fmt.Errorf("file menu did not open YAML editor")
	}
	return nil
}
