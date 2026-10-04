package main

import (
	"testing"
	"time"
)

func TestSlaveScopeAndReadEvidence(t *testing.T) {
	w := newWorld()
	g := w.Gateways[1]
	ds := g.Downstreams[1]
	for i, id := range []byte{9, 10} {
		x := Exchange{Gw: g, Ds: ds}
		x.SlaveID = id
		x.Time = time.Now()
		x.FunctionCode = 3
		x.Address = 0
		x.Quantity = 1
		x.Response = []byte{3, 2, 0, byte(11 + i)}
		w.record(x)
	}
	x := Exchange{Gw: g, Ds: ds}
	x.SlaveID = 9
	x.FunctionCode = 6
	x.Address = 0
	x.Quantity = 1
	x.Request = []byte{6, 0, 0, 0, 99}
	x.Response = x.Request
	w.record(x)
	l := Link{Gw: g, Ds: ds, SlaveFilter: 10}
	events := w.Exchanges(l.Match)
	if len(events) != 2 {
		t.Fatalf("ID 9 scope: got %d exchanges", len(events))
	}
	vals, _ := observed(events, holding)
	if vals[0] != 11 {
		t.Fatalf("write echo replaced last read: %d", vals[0])
	}
	l.SlaveFilter = 11
	vals, _ = observed(w.Exchanges(l.Match), holding)
	if vals[0] != 12 {
		t.Fatalf("ID 10 value mixed with ID 9: %d", vals[0])
	}
}

func TestInjectorDestinationAndHistoricValues(t *testing.T) {
	w := newWorld()
	ds := w.Gateways[0].Downstreams[1]
	s := w.sim(ds.Sim)
	original := s.Values[holding][3]
	x := Exchange{Ds: ds}
	x.FunctionCode = 16
	x.Address = 3
	x.Quantity = 2
	x.Request = []byte{16, 0, 3, 0, 2, 4, 0, 25, 0, 26}
	x.Response = []byte{16, 0, 3, 0, 2}
	x.Time = time.Now()
	w.inject(&x, s)
	tab, start, count, ok := affectedRange(&x)
	if !ok || tab != input || start != 63 || count != 2 {
		t.Fatalf("wrong mapping: %v %d %d %d", ok, tab, start, count)
	}
	if s.Values[input][63] != 25 || s.Values[input][64] != 26 || s.Values[holding][3] != original {
		t.Fatal("injection must update destination only")
	}
	if !touches(&x, input, 63) || touches(&x, holding, 3) {
		t.Fatal("request linkage must follow destination")
	}
	s.Values[input][63] = 77
	if requestValues(&x)[0] != 25 {
		t.Fatal("historic request value changed with current model")
	}
	x.Address = 39
	w.inject(&x, s)
	if x.Err == nil || x.Response[1] != 2 {
		t.Fatal("cross-boundary write must fail")
	}
}

func TestPacketAbsenceDistinguishesFailureAndInProcess(t *testing.T) {
	w := newWorld()
	cases := []struct {
		x          Exchange
		name, want string
	}{
		{Exchange{}, "下游请求", "没有下游调用（无路由）"},
		{Exchange{Ds: w.Gateways[0].Downstreams[0]}, "下游请求", "没有线路帧（进程内调用）"},
		{Exchange{Ds: w.Gateways[0].Downstreams[2], DownErr: "超时"}, "下游应答", "没有响应：超时"},
		{Exchange{Ds: w.Gateways[1].Downstreams[1]}, "下游应答", "未采集原始帧"},
	}
	for _, c := range cases {
		if got := packetAbsence(&c.x, c.name); got != c.want {
			t.Fatalf("got %q want %q", got, c.want)
		}
	}
}
