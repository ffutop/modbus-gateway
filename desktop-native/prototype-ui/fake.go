// PROTOTYPE — throwaway. An in-memory stand-in for the gateways: fixed
// topology, simulated register tables and a generator of plausible traffic
// in which every request has an upstream leg and a downstream leg.

package main

import (
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"

	"fmt"
	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
	"github.com/ffutop/modbus-gateway/modbus/crc"
)

type table int

const (
	holding table = iota
	input
	coils
	discrete
)

var tableNames = []string{"保持寄存器 4x", "输入寄存器 3x", "线圈 0x", "离散输入 1x"}

const tableSize = 120

type Sim struct {
	Name    string
	Persist string
	Values  [4][tableSize]uint16 // coils/discrete store 0/1
	Changed [4][tableSize]time.Time
	Writes  int
}

type Downstream struct {
	Name     string
	Type     string // local, injector, tcp, rtu
	Target   string // address or device; empty for simulations
	Slave    byte
	SlaveIDs string
	Slaves   []byte
	Mappings []config.MappingConfig
	Sim      string // simulation name for local/injector
	Online   bool
	Gateway  *Gateway
}

func (d *Downstream) RouteIDs() string {
	if d.SlaveIDs != "" {
		return d.SlaveIDs
	}
	return fmt.Sprint(d.Slave)
}
func (d *Downstream) IDs() []byte {
	if len(d.Slaves) > 0 {
		return d.Slaves
	}
	return []byte{d.Slave}
}

func (d *Downstream) InProcess() bool { return d.Sim != "" }

// Proto names the downstream framing.
func (d *Downstream) Proto() string {
	switch d.Type {
	case "tcp":
		return "Modbus TCP"
	case "rtu":
		return "Modbus RTU"
	}
	return "进程内"
}

type Gateway struct {
	Name        string
	UpType      string // tcp, rtu-over-tcp, rtu
	UpAddr      string
	Masters     []string // upstream peers seen on this gateway
	Downstreams []*Downstream
}

func (g *Gateway) UpProto() string {
	switch g.UpType {
	case "rtu-over-tcp":
		return "RTU over TCP"
	case "rtu":
		return "Modbus RTU"
	}
	return "Modbus TCP"
}

// Exchange is one request through a gateway: the upstream leg (master ↔
// gateway) and the downstream leg (gateway ↔ device or model).
type Exchange struct {
	telemetry.Event // Source = master; Request/Response = PDUs

	Gw       *Gateway
	Ds       *Downstream // nil when no route matched
	UpReq    []byte      // ADUs on the upstream wire
	UpResp   []byte
	DownReq  []byte // ADUs on the downstream wire; nil for in-process models
	DownResp []byte
	SentAt   time.Duration // downstream request sent, relative to Time
	RecvAt   time.Duration // downstream response received (or gave up)
	DownErr  string        // downstream failure, e.g. timeout
}

// Link is a path through a gateway; empty fields mean "any". Sim selects
// every downstream backed by that simulation, across gateways.
type Link struct {
	Gw          *Gateway
	Master      string
	Ds          *Downstream
	Sim         string
	SlaveFilter int // 0 = all; otherwise Slave ID + 1 (including ID 0).
}

func (l Link) Match(x *Exchange) bool {
	return (l.Gw == nil || x.Gw == l.Gw) && (l.Master == "" || x.Source == l.Master) && (l.Ds == nil || x.Ds == l.Ds) &&
		(l.Sim == "" || x.Ds != nil && x.Ds.Sim == l.Sim) && (l.SlaveFilter == 0 || int(x.SlaveID)+1 == l.SlaveFilter)
}

type World struct {
	random    *rand.Rand
	mu        sync.RWMutex
	Gateways  []*Gateway
	Sims      []*Sim
	exchanges []Exchange
	seq       uint64
	tid       uint16
	counts    map[Link]*[2]uint64 // requests, errors
	rates     map[Link]float64
	lastRate  map[Link]uint64
	PausedTo  time.Time // traffic stops until then (simulated restart)
}

const maxExchanges = 5000

func newWorld() *World {
	random := rand.New(rand.NewSource(19))
	boiler := &Sim{Name: "锅炉房", Persist: "file · data/boiler.json"}
	conveyor := &Sim{Name: "输送线", Persist: "memory"}
	for i := 0; i < tableSize; i++ {
		boiler.Values[holding][i] = uint16(random.Intn(500))
		conveyor.Values[holding][i] = uint16(random.Intn(100))
		boiler.Values[coils][i] = uint16(random.Intn(2))
	}
	w := &World{
		random:   random,
		Sims:     []*Sim{boiler, conveyor},
		counts:   map[Link]*[2]uint64{},
		rates:    map[Link]float64{},
		lastRate: map[Link]uint64{},
	}
	g1 := &Gateway{Name: "产线 1", UpType: "tcp", UpAddr: "0.0.0.0:502", Masters: []string{"SCADA 10.0.3.17", "HMI 10.0.3.22"}}
	g1.Downstreams = []*Downstream{
		{Name: "锅炉 PLC", Type: "local", Slave: 1, Sim: "锅炉房", Online: true},
		{Name: "锅炉传感器注入", Type: "injector", Slave: 2, Sim: "锅炉房", Online: true, Mappings: []config.MappingConfig{{Source: config.MappingSourceConfig{Table: "holding_registers", StartAddress: 0, Count: 40}, Target: config.MappingTargetConfig{Table: "input_registers", StartAddress: 60}}}},
		{Name: "变频器", Type: "rtu", Target: "COM3 · 19200 8N1", Slave: 5, Online: false},
	}
	g2 := &Gateway{Name: "仓储", UpType: "rtu-over-tcp", UpAddr: "0.0.0.0:4001", Masters: []string{"WMS 10.0.4.8"}}
	g2.Downstreams = []*Downstream{
		{Name: "输送线", Type: "local", Slave: 1, Sim: "输送线", Online: true},
		{Name: "称重仪", Type: "tcp", Target: "192.168.10.21:502", Slave: 9, SlaveIDs: "9,10", Slaves: []byte{9, 10}, Online: true},
	}
	w.Gateways = []*Gateway{g1, g2}
	for _, g := range w.Gateways {
		for _, d := range g.Downstreams {
			d.Gateway = g
		}
	}
	return w
}

func (w *World) sim(name string) *Sim {
	for _, s := range w.Sims {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (w *World) run(stop <-chan struct{}, onChange func()) {
	tick := time.NewTicker(25 * time.Millisecond)
	rateTick := time.NewTicker(time.Second)
	redraw := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	defer rateTick.Stop()
	defer redraw.Stop()
	start := time.Now()
	for {
		select {
		case <-stop:
			return
		case now := <-tick.C:
			w.mu.Lock()
			if now.After(w.PausedTo) {
				w.drift(now, now.Sub(start).Seconds())
				for n := w.random.Intn(3); n > 0; n-- {
					w.request(now)
				}
			}
			w.mu.Unlock()
		case <-rateTick.C:
			w.mu.Lock()
			for k, c := range w.counts {
				w.rates[k] = float64(c[0] - w.lastRate[k])
				w.lastRate[k] = c[0]
			}
			w.mu.Unlock()
		case <-redraw.C:
			onChange()
		}
	}
}

func (w *World) drift(now time.Time, t float64) {
	s := w.Sims[0]
	for i := 0; i < 6; i++ {
		v := uint16(500 + 300*math.Sin(t/3+float64(i)) + w.random.Float64()*8)
		if w.random.Intn(8) == 0 && v != s.Values[input][i] {
			s.Values[input][i] = v
			s.Changed[input][i] = now
		}
	}
}

func (w *World) request(now time.Time) {
	g := w.Gateways[w.random.Intn(len(w.Gateways))]
	x := Exchange{Gw: g}
	x.Time, x.Gateway = now, g.Name
	x.Source = g.Masters[w.random.Intn(len(g.Masters))]
	slave := byte(0)
	if w.random.Intn(30) == 0 { // a slave with no route
		slave = 77
	} else {
		x.Ds = g.Downstreams[w.random.Intn(len(g.Downstreams))]
		ids := x.Ds.IDs()
		slave = ids[w.random.Intn(len(ids))]
		x.Downstream = x.Ds.Name
	}
	x.SlaveID = slave
	var s *Sim
	if x.Ds != nil {
		s = w.sim(x.Ds.Sim)
	}
	injecting := x.Ds != nil && x.Ds.Type == "injector"
	if injecting {
		w.op(&x.Event, nil, true)
		w.inject(&x, s)
	} else {
		w.op(&x.Event, s, false)
	}

	upTid := uint16(w.random.Intn(0xFFFF))
	x.UpReq = w.frame(g.UpType, upTid, slave, x.Request)
	switch {
	case x.Ds == nil:
		x.Err = errors.New("gateway path unavailable")
		x.Response = []byte{x.FunctionCode | 0x80, 0x0A}
		x.Duration = 40 * time.Microsecond
	case x.Ds.InProcess():
		x.SentAt, x.RecvAt = 30*time.Microsecond, 30*time.Microsecond+time.Duration(20+w.random.Intn(60))*time.Microsecond
		x.Duration = x.RecvAt + 25*time.Microsecond
	default:
		w.tid++
		x.DownReq = w.frame(x.Ds.Type, w.tid, slave, x.Request)
		x.SentAt = time.Duration(40+w.random.Intn(40)) * time.Microsecond
		if !x.Ds.Online {
			x.RecvAt = x.SentAt + 2*time.Second
			x.DownErr = "超时：2.00 s 内无应答"
			x.Err = errors.New("modbus: request timed out")
			x.Response = []byte{x.FunctionCode | 0x80, 0x0B}
		} else {
			x.RecvAt = x.SentAt + time.Duration(800+w.random.Intn(2500))*time.Microsecond
			x.DownResp = w.frame(x.Ds.Type, w.tid, slave, x.Response)
		}
		x.Duration = x.RecvAt + 30*time.Microsecond
	}
	x.UpResp = w.frame(g.UpType, upTid, slave, x.Response)
	w.record(x)
}

// frame wraps pdu for the wire: an MBAP header for TCP, address + CRC for
// RTU and RTU over TCP.
func (w *World) frame(kind string, tid uint16, slave byte, pdu []byte) []byte {
	if len(pdu) == 0 {
		return nil
	}
	if kind == "tcp" {
		out := make([]byte, 7, 7+len(pdu))
		binary.BigEndian.PutUint16(out[0:], tid)
		binary.BigEndian.PutUint16(out[4:], uint16(len(pdu)+1))
		out[6] = slave
		return append(out, pdu...)
	}
	out := append([]byte{slave}, pdu...)
	var c crc.CRC
	sum := c.Reset().PushBytes(out).Value()
	return append(out, byte(sum), byte(sum>>8))
}

// op fills in a random request PDU and, against s, its response PDU.
func (w *World) op(e *telemetry.Event, s *Sim, injecting bool) {
	start := uint16(w.random.Intn(30))
	qty := uint16(1 + w.random.Intn(8))
	switch r := w.random.Intn(10); {
	case r < 5:
		e.FunctionCode = 3
	case r < 6:
		e.FunctionCode = 4
	case r < 7:
		e.FunctionCode = 1
	case r < 9:
		e.FunctionCode = 6
		qty = 1
	default:
		e.FunctionCode = 16
		qty = uint16(1 + w.random.Intn(3))
	}
	if injecting {
		e.FunctionCode = 16
		qty = 2
	}
	e.Address, e.Quantity = start, qty
	fc := e.FunctionCode
	req := []byte{fc, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(req[1:], start)
	binary.BigEndian.PutUint16(req[3:], qty)
	e.Request = req
	switch fc {
	case 1:
		n := (int(qty) + 7) / 8
		resp := []byte{fc, byte(n)}
		for b := 0; b < n; b++ {
			var v byte
			for bit := 0; bit < 8 && b*8+bit < int(qty); bit++ {
				if s != nil && s.Values[coils][int(start)+b*8+bit] != 0 || s == nil && w.random.Intn(2) == 0 {
					v |= 1 << bit
				}
			}
			resp = append(resp, v)
		}
		e.Response = resp
	case 3, 4:
		t := holding
		if fc == 4 {
			t = input
		}
		resp := []byte{fc, byte(qty * 2)}
		for i := 0; i < int(qty); i++ {
			v := uint16(w.random.Intn(2000))
			if s != nil {
				v = s.Values[t][int(start)+i]
			}
			resp = binary.BigEndian.AppendUint16(resp, v)
		}
		e.Response = resp
	case 6:
		v := uint16(w.random.Intn(1000))
		binary.BigEndian.PutUint16(req[3:], v)
		if s != nil {
			s.Values[holding][start], s.Changed[holding][start] = v, e.Time
			s.Writes++
		}
		e.Response = append([]byte(nil), req...)
	case 16:
		req = append(req, byte(qty*2))
		for i := 0; i < int(qty); i++ {
			v := uint16(w.random.Intn(1000))
			req = binary.BigEndian.AppendUint16(req, v)
			if s != nil {
				s.Values[holding][int(start)+i], s.Changed[holding][int(start)+i] = v, e.Time
			}
		}
		if s != nil {
			s.Writes++
		}
		e.Request, e.Response = req, append([]byte(nil), req[:5]...)
	}
}

func (w *World) record(x Exchange) {
	w.seq++
	x.Seq = w.seq
	if len(w.exchanges) >= maxExchanges {
		w.exchanges = append(w.exchanges[:0], w.exchanges[500:]...)
	}
	w.exchanges = append(w.exchanges, x)
	keys := []Link{{Gw: x.Gw}, {Gw: x.Gw, Master: x.Source}, {Gw: x.Gw, Ds: x.Ds}, {Gw: x.Gw, Master: x.Source, Ds: x.Ds}}
	for _, k := range keys {
		c := w.counts[k]
		if c == nil {
			c = new([2]uint64)
			w.counts[k] = c
		}
		c[0]++
		if x.Err != nil {
			c[1]++
		}
	}
}

// Exchanges returns copies of the exchanges matching keep, oldest first.
func (w *World) Exchanges(keep func(*Exchange) bool) []Exchange {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]Exchange, 0, 256)
	for i := range w.exchanges {
		if keep == nil || keep(&w.exchanges[i]) {
			out = append(out, w.exchanges[i])
		}
	}
	return out
}

func (w *World) Counts(l Link) (requests, errs uint64, rate float64) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if c := w.counts[l]; c != nil {
		requests, errs = c[0], c[1]
	}
	return requests, errs, w.rates[l]
}

func (w *World) Snapshot(simName string, t table) (vals [tableSize]uint16, changed [tableSize]time.Time) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if s := w.sim(simName); s != nil {
		return s.Values[t], s.Changed[t]
	}
	return
}

// Restart pauses traffic briefly, as an in-process gateway restart would.
func (w *World) Restart() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.PausedTo = time.Now().Add(1200 * time.Millisecond)
}

func (w *World) Restarting(now time.Time) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return now.Before(w.PausedTo)
}

func tableOf(fc byte) (table, bool) {
	switch fc & 0x7F {
	case 1, 5, 15:
		return coils, true
	case 2:
		return discrete, true
	case 3, 6, 16, 23:
		return holding, true
	case 4:
		return input, true
	}
	return 0, false
}

func isWrite(fc byte) bool {
	switch fc & 0x7F {
	case 5, 6, 15, 16, 23:
		return true
	}
	return false
}

// affectedRange translates injector source addresses to their model destination.
func affectedRange(x *Exchange) (table, int, int, bool) {
	t, ok := tableOf(x.FunctionCode)
	if !ok {
		return 0, 0, 0, false
	}
	start, count := int(x.Address), int(x.Quantity)
	if x.Ds != nil && x.Ds.Type == "injector" {
		for _, m := range x.Ds.Mappings {
			if m.Source.Table != "holding_registers" && t == holding || m.Source.Table != "coils" && t == coils {
				continue
			}
			if t != holding && t != coils {
				continue
			}
			if start >= int(m.Source.StartAddress) && start+count <= int(m.Source.StartAddress)+int(m.Source.Count) {
				target := input
				if m.Target.Table == "discrete_inputs" {
					target = discrete
				}
				a := int(m.Target.StartAddress) + start - int(m.Source.StartAddress)
				return target, a, count, true
			}
		}
		return 0, 0, 0, false
	}
	return t, start, count, true
}

func requestValues(x *Exchange) []uint16 {
	if x.Err != nil || len(x.Response) == 0 || x.Response[0]&0x80 != 0 {
		return nil
	}
	vals := make([]uint16, 0, x.Quantity)
	for i := 0; i < int(x.Quantity); i++ {
		switch x.FunctionCode {
		case 3, 4:
			off := 2 + 2*i
			if off+1 >= len(x.Response) {
				return vals
			}
			vals = append(vals, binary.BigEndian.Uint16(x.Response[off:]))
		case 1, 2:
			off := 2 + i/8
			if off >= len(x.Response) {
				return vals
			}
			vals = append(vals, uint16((x.Response[off]>>uint(i%8))&1))
		case 6:
			if len(x.Request) < 5 {
				return nil
			}
			vals = append(vals, binary.BigEndian.Uint16(x.Request[3:5]))
		case 5:
			if len(x.Request) < 5 {
				return nil
			}
			v := uint16(0)
			if x.Request[3] == 0xff {
				v = 1
			}
			vals = append(vals, v)
		case 16:
			off := 6 + 2*i
			if off+1 >= len(x.Request) {
				return vals
			}
			vals = append(vals, binary.BigEndian.Uint16(x.Request[off:]))
		case 15:
			off := 6 + i/8
			if off >= len(x.Request) {
				return vals
			}
			vals = append(vals, uint16((x.Request[off]>>uint(i%8))&1))
		}
	}
	return vals
}

func (w *World) inject(x *Exchange, s *Sim) {
	t, a, _, ok := affectedRange(x)
	if !ok || s == nil {
		x.Err = errors.New("注入范围未映射")
		x.Response = []byte{x.FunctionCode | 0x80, 2}
		return
	}
	for i, v := range requestValues(x) {
		if a+i < tableSize {
			s.Values[t][a+i] = v
			s.Changed[t][a+i] = x.Time
		}
	}
	s.Writes++
}
