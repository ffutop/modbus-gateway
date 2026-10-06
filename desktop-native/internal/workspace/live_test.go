package workspace

import (
	"context"
	"errors"
	"fmt"
	"image"
	"net"
	"strings"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	gwapp "github.com/ffutop/modbus-gateway/internal/app"
	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/modbus"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
	"github.com/ffutop/modbus-gateway/internal/transport/tcp"
)

func liveConfig(address string) string {
	return fmt.Sprintf(`version: 1
# retain notes
simulations:
  - name: model
    persistence: {type: memory}
gateways:
  - name: demo
    upstreams:
      - type: tcp
        tcp: {address: %q}
    downstreams:
      - name: local
        type: local
        slave_ids: "1"
        simulation: {ref: model}
      - name: injector
        type: injector
        slave_ids: "2"
        simulation:
          ref: model
          mappings:
            - source: {table: holding_registers, start_address: 0, count: 4}
              target: {table: input_registers, start_address: 16}
      - name: device
        type: tcp
        slave_ids: "9,10"
        tcp: {address: "127.0.0.1:15021"}
`, address)
}

func parsed(t *testing.T, text string) *config.Config {
	t.Helper()
	c, err := config.ParseDraft([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if p := c.Problems(); len(p) > 0 {
		t.Fatal(p)
	}
	return c
}

func TestLiveProjectionRetainsPerSlaveReadEvidence(t *testing.T) {
	c := parsed(t, liveConfig("127.0.0.1:15020"))
	r := telemetry.NewRecorder(8)
	w := newLiveWorld(c, live.Local{Recorder: r})
	g, d := w.Gateways[0], w.Gateways[0].Downstreams[2]
	if d.Observed {
		t.Fatal("configured device presented as observed")
	}
	base := time.Now()
	read := func(id byte, value byte) telemetry.Event {
		return telemetry.Event{Time: base, Gateway: "demo", Downstream: "device", Source: "master", SlaveID: id, FunctionCode: 3, Quantity: 1, Request: []byte{3, 0, 0, 0, 1}, Response: []byte{3, 2, 0, value}}
	}
	r.Record(read(9, 25))
	r.Record(read(10, 40))
	w.Poll(base)
	l := Link{Gw: g, Ds: d, SlaveFilter: 10}
	values, seen := w.Observed(l, holding)
	if values[0] != 25 || seen[0].IsZero() {
		t.Fatal("slave 9 read lost")
	}
	l.SlaveFilter = 11
	values, _ = w.Observed(l, holding)
	if values[0] != 40 {
		t.Fatal("slave IDs mixed")
	}
	write := read(9, 99)
	write.FunctionCode = 6
	write.Request = []byte{6, 0, 0, 0, 99}
	write.Response = write.Request
	r.Record(write)
	bad := read(9, 88)
	bad.Err = errors.New("timeout")
	bad.Response = nil
	r.Record(bad)
	w.Poll(base.Add(time.Second))
	w.exchanges = nil // eviction must not clear the last successful read
	l.SlaveFilter = 10
	values, _ = w.Observed(l, holding)
	if values[0] != 25 {
		t.Fatal("write echo/failure replaced a confirmed read")
	}
	if !d.Observed || d.Online {
		t.Fatal("last failed result not reflected")
	}
	if n, _, _ := w.Counts(Link{Gw: g}); n != 4 {
		t.Fatalf("gateway total = %d", n)
	}
}

func TestLiveProjectionBoundsHistoryAndReportsLoss(t *testing.T) {
	w := newLiveWorld(parsed(t, liveConfig("127.0.0.1:15020")), nil)
	for i := 0; i < maxExchanges+20; i++ {
		w.record(telemetry.Event{Seq: uint64(i + 1), Gateway: "demo", Source: fmt.Sprint(i), FunctionCode: 3})
	}
	if len(w.exchanges) != maxExchanges || len(w.Gateways[0].Masters) != 64 {
		t.Fatal("unbounded history or master list")
	}
	if w.Exchanges(nil)[0].Seq != 21 {
		t.Fatal("wrong eviction order")
	}
	r := telemetry.NewRecorder(2)
	w = newLiveWorld(parsed(t, liveConfig("127.0.0.1:15020")), live.Local{Recorder: r})
	for i := 0; i < 5; i++ {
		r.Record(telemetry.Event{Gateway: "demo"})
	}
	w.Poll(time.Now())
	if w.missed != 3 {
		t.Fatalf("missed = %d", w.missed)
	}
}

func TestDesktopReadsRealLocalAndInjectorTraffic(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	l.Close()
	text := liveConfig(address)
	// Only in-process downstreams are needed for this integration test.
	c := parsed(t, text)
	c.Gateways[0].Downstreams = c.Gateways[0].Downstreams[:2]
	r := telemetry.NewRecorder(100)
	a, err := gwapp.New(c, r)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.Start(ctx)
	defer func() { cancel(); a.Wait(); a.Close() }()
	client := tcp.NewClient(address)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err = client.Connect(ctx); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer client.Close()
	request := modbus.ProtocolDataUnit{FunctionCode: 6, Data: []byte{0, 0, 0, 25}}
	if _, err := client.Send(ctx, 1, request); err != nil {
		t.Fatal(err)
	}
	request.Data = []byte{0, 0, 0, 77}
	if _, err := client.Send(ctx, 2, request); err != nil {
		t.Fatal(err)
	}
	request = modbus.ProtocolDataUnit{FunctionCode: 4, Data: []byte{0, 16, 0, 1}}
	if _, err := client.Send(ctx, 1, request); err != nil {
		t.Fatal(err)
	}
	w := newLiveWorld(c, live.Local{Recorder: r, Simulations: a.Simulations})
	w.Poll(time.Now())
	xs := w.Exchanges(nil)
	if len(xs) != 3 || xs[0].Ds.Name != "local" || xs[1].Ds.Name != "injector" || xs[2].Source == "" {
		t.Fatalf("bad real telemetry: %+v", xs)
	}
	values, _, ok := w.Snapshot("model", input)
	if !ok || values[16] != 77 {
		t.Fatalf("injector target = %d", values[16])
	}
	values, _, _ = w.Snapshot("model", holding)
	if values[0] != 25 {
		t.Fatal("injector modified source table")
	}
	for _, x := range xs {
		if len(x.UpReq)+len(x.UpResp)+len(x.DownReq)+len(x.DownResp) > 0 || x.SentAt != 0 || x.RecvAt != 0 {
			t.Fatal("invented wire frame or timing")
		}
	}
}

func TestEditorSaveFailureKeepsDraftAndRunningConfig(t *testing.T) {
	text := liveConfig("127.0.0.1:15020")
	c := parsed(t, text)
	info := Info{Config: c, Content: text, Running: true, Save: func(string) error { return errors.New("conflict") }}
	e := newConfigEditor(NewTheme(), info)
	e.draft.Gateways[0].Name = "changed"
	if c.Gateways[0].Name != "demo" {
		t.Fatal("draft mutated running configuration")
	}
	if err := e.doSave(); err == nil {
		t.Fatal("save failure hidden")
	}
	if e.savedYAML != text || e.runningYAML != text || len(e.changes()) == 0 {
		t.Fatal("save failure discarded draft or baseline")
	}
	var saved string
	e.saveFile = func(s string) error { saved = s; return nil }
	if err := e.doSave(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saved, "retain notes") || len(e.changes()) != 0 || e.runningYAML != text || len(e.pendingChanges()) == 0 {
		t.Fatal("save changed runtime or lost pending changes")
	}
}

func TestStartupFailureAndEmptyConfigLayout(t *testing.T) {
	for _, text := range []string{"version: [", "gateways: []\n"} {
		u := New(Info{Content: text, StartErr: errors.New("startup failed")})
		if u.view.shell.module != 1 || !u.view.shell.cfg.raw {
			t.Fatal("startup failure must open YAML editor")
		}
		var ops op.Ops
		u.Layout(layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1100, 680))})
	}
}

func TestTopologyFoldsMastersUntilOneIsChosen(t *testing.T) {
	w := newLiveWorld(parsed(t, liveConfig("127.0.0.1:15020")), nil)
	g := w.Gateways[0]
	before := (&topology{world: w}).height(g)
	for i := 0; i < 40; i++ {
		w.record(telemetry.Event{Seq: uint64(i + 1), Gateway: "demo", Source: fmt.Sprintf("10.0.0.1:%d", 40000+i), FunctionCode: 3})
	}
	tp := &topology{th: NewTheme(), world: w, btns: clicks[string]{}}
	if tp.height(g) != before {
		t.Fatal("masters grow the topology")
	}
	masters := func(ns []*topoNode) (n int, first *topoNode) {
		for _, x := range ns {
			if x.col == 0 {
				n++
				if first == nil {
					first = x
				}
			}
		}
		return n, first
	}
	if n, m := masters(tp.nodes(g, "")); n != 1 || !m.placeholder {
		t.Fatal("masters not folded into a placeholder")
	}
	if n, m := masters(tp.nodes(g, "10.0.0.1:40003")); n != 1 || m.placeholder || m.title != "10.0.0.1:40003" {
		t.Fatal("chosen master not shown")
	}
	if l := click(Link{Gw: g}, g, &topoNode{col: 0, placeholder: true}); l.Master != "" {
		t.Fatal("placeholder click filtered by master")
	}
}

func TestRequestListFillsFromTop(t *testing.T) {
	w := newLiveWorld(parsed(t, liveConfig("127.0.0.1:15020")), nil)
	v := newLinkedView(NewTheme(), w)
	l := Link{Gw: w.Gateways[0]}
	frame := func() {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1000, 500))}
		v.Layout(gtx, l, false, func(gtx C, x *Exchange) D { return D{} })
	}
	for i := 0; i < 3; i++ {
		w.record(telemetry.Event{Seq: uint64(i + 1), Gateway: "demo", Downstream: "local", FunctionCode: 3, Quantity: 1})
	}
	frame()
	frame()
	if p := v.traffic.Position; p.First != 0 || p.Offset != 0 || !v.traffic.ScrollToEnd {
		t.Fatalf("short list not top aligned: %+v", p)
	}
	for i := 3; i < 200; i++ {
		w.record(telemetry.Event{Seq: uint64(i + 1), Gateway: "demo", Downstream: "local", FunctionCode: 3, Quantity: 1})
	}
	frame()
	if p := v.traffic.Position; p.First == 0 || p.BeforeEnd {
		t.Fatalf("long list stopped following: %+v", p)
	}
}
