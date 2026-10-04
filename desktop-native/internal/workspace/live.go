package workspace

import (
	"encoding/binary"
	"fmt"
	"time"

	"gioui.org/op"
	"gioui.org/op/paint"
	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/routing"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

const maxExchanges = 5000

// World is a UI-owned projection of the running configuration and recorder.
// Poll and Layout run on the window goroutine; simulations have their own locks.
type World struct {
	Gateways     []*Gateway
	Sims         []*Sim
	models       map[string]*simulation.Simulation
	recorder     *telemetry.Recorder
	exchanges    []Exchange
	head         int // next overwritten slot once history reaches capacity
	cursor       uint64
	missed       uint64
	lastPoll     time.Time
	counts       map[Link][2]uint64
	rates        map[Link]float64
	observations map[Link]*observation
}

type observation struct {
	values [4][tableSize]uint16
	seen   [4][tableSize]time.Time
}

func newLiveWorld(cfg *config.Config, models map[string]*simulation.Simulation, rec *telemetry.Recorder) *World {
	w := &World{models: models, recorder: rec, counts: map[Link][2]uint64{}, rates: map[Link]float64{}, observations: map[Link]*observation{}}
	for _, s := range cfg.Simulations {
		persist := s.Persistence.Type
		if s.Persistence.Path != "" {
			persist += " · " + s.Persistence.Path
		}
		w.Sims = append(w.Sims, &Sim{Name: s.Name, Persist: persist})
	}
	for _, gc := range cfg.Gateways {
		g := &Gateway{Name: gc.Name}
		for i, u := range gc.Upstreams {
			if i == 0 {
				g.UpType = u.Type
			} else if u.Type != g.UpType {
				g.UpType = "多种协议"
			}
			if i > 0 {
				g.UpAddr += " / "
			}
			if u.Type == "rtu" {
				g.UpAddr += u.Serial.Device
			} else {
				g.UpAddr += u.Tcp.Address
			}
		}
		for i, dc := range gc.Downstreams {
			ids, _ := routing.ParseSlaveIDs(dc.SlaveIDs)
			// An empty expression on the sole downstream is the legacy default route.
			if len(gc.Downstreams) == 1 && dc.SlaveIDs == "" {
				for id := 0; id <= 255; id++ {
					ids = append(ids, byte(id))
				}
			}
			d := &Downstream{Name: dc.DisplayName(i), Type: dc.Type, SlaveIDs: dc.SlaveIDs, Slaves: ids, Sim: dc.SimulationRef, Mappings: dc.Mappings, Gateway: g}
			if len(ids) > 0 {
				d.Slave = ids[0]
			}
			if dc.Type == "rtu" {
				d.Target = fmt.Sprintf("%s · %d", dc.Serial.Device, dc.Serial.BaudRate)
			} else {
				d.Target = dc.Tcp.Address
			}
			g.Downstreams = append(g.Downstreams, d)
		}
		w.Gateways = append(w.Gateways, g)
	}
	return w
}

func (w *World) Poll(now time.Time) {
	previous := make(map[Link][2]uint64, len(w.counts))
	for k, v := range w.counts {
		previous[k] = v
	}
	for _, e := range w.recorder.Since(w.cursor) {
		if e.Seq <= w.cursor {
			continue
		}
		if e.Seq > w.cursor+1 {
			w.missed += e.Seq - w.cursor - 1
		}
		w.cursor = e.Seq
		w.record(e)
	}
	if dt := now.Sub(w.lastPoll).Seconds(); !w.lastPoll.IsZero() && dt > 0 {
		for k, c := range w.counts {
			w.rates[k] = float64(c[0]-previous[k][0]) / dt
		}
	}
	w.lastPoll = now
}

func (w *World) record(e telemetry.Event) {
	var g *Gateway
	for _, candidate := range w.Gateways {
		if candidate.Name == e.Gateway {
			g = candidate
			break
		}
	}
	if g == nil {
		return
	}
	x := Exchange{Event: e, Gw: g}
	for _, d := range g.Downstreams {
		if e.Downstream != "" && d.Name == e.Downstream {
			x.Ds = d
			break
		}
	}
	if x.Ds != nil {
		x.Ds.Observed = true
		x.Ds.Online = !failed(&e)
	}
	if e.Source != "" {
		found := false
		for _, m := range g.Masters {
			if m == e.Source {
				found = true
				break
			}
		}
		if !found && len(g.Masters) < 64 {
			g.Masters = append(g.Masters, e.Source)
		}
	}
	// No synthetic ADU, transaction ID, CRC, exception reply, or leg timing.
	if e.Err != nil {
		x.DownErr = e.Err.Error()
	}
	if len(w.exchanges) == maxExchanges {
		w.exchanges[w.head] = x
		w.head = (w.head + 1) % maxExchanges
	} else {
		w.exchanges = append(w.exchanges, x)
	}
	keys := []Link{{Gw: g}, {Gw: g, Ds: x.Ds}}
	for _, m := range g.Masters {
		if m == e.Source {
			keys = append(keys, Link{Gw: g, Master: m}, Link{Gw: g, Master: m, Ds: x.Ds})
			break
		}
	}
	if x.Ds != nil && x.Ds.Sim != "" {
		keys = append(keys, Link{Sim: x.Ds.Sim}, Link{Gw: g, Sim: x.Ds.Sim})
	}
	// Count the empty-downstream gateway key just once.
	seen := map[Link]bool{}
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		c := w.counts[k]
		c[0]++
		if failed(&e) {
			c[1]++
		}
		w.counts[k] = c
	}
	if x.Ds == nil || x.Ds.InProcess() || isWrite(e.FunctionCode) || failed(&e) {
		return
	}
	t, addr, _, ok := affectedRange(&x)
	if !ok {
		return
	}
	k := Link{Gw: g, Ds: x.Ds, SlaveFilter: int(e.SlaveID) + 1}
	o := w.observations[k]
	if o == nil {
		o = &observation{}
		w.observations[k] = o
	}
	for i, value := range requestValues(&x) {
		if addr+i < tableSize {
			o.values[t][addr+i] = value
			o.seen[t][addr+i] = e.Time
		}
	}
}

func (w *World) sim(name string) *Sim {
	for _, s := range w.Sims {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (w *World) Exchanges(keep func(*Exchange) bool) []Exchange {
	out := make([]Exchange, 0, len(w.exchanges))
	for i := range w.exchanges {
		x := &w.exchanges[(w.head+i)%len(w.exchanges)]
		if keep == nil || keep(x) {
			out = append(out, *x)
		}
	}
	return out
}

func (w *World) Counts(l Link) (uint64, uint64, float64) {
	if c, ok := w.counts[l]; ok {
		return c[0], c[1], w.rates[l]
	}
	var count, errs uint64
	for i := range w.exchanges {
		x := &w.exchanges[i]
		if l.Match(x) {
			count++
			if failed(&x.Event) {
				errs++
			}
		}
	}
	return count, errs, 0 // per-master counts describe retained history, no invented rate
}

func (w *World) Observed(l Link, t table) (values [tableSize]uint16, seen [tableSize]time.Time) {
	l.Master, l.Sim = "", ""
	if o := w.observations[l]; o != nil {
		return o.values[t], o.seen[t]
	}
	return
}

func (w *World) Snapshot(name string, t table) (values [tableSize]uint16, changed [tableSize]time.Time) {
	s := w.models[name]
	if s == nil {
		return
	}
	var raw []byte
	var err error
	switch t {
	case holding:
		raw, err = s.Model.ReadHoldingRegisters(0, tableSize)
	case input:
		raw, err = s.Model.ReadInputRegisters(0, tableSize)
	case coils:
		raw, err = s.Model.ReadCoils(0, tableSize)
	case discrete:
		raw, err = s.Model.ReadDiscreteInputs(0, tableSize)
	}
	if err != nil {
		return
	}
	for i := range values {
		if t == coils || t == discrete {
			values[i] = uint16(raw[i/8]>>uint(i%8)) & 1
		} else {
			values[i] = binary.BigEndian.Uint16(raw[i*2:])
		}
	}
	return // model does not expose per-address write times
}

// UI is the production workspace. It never generates traffic.
type UI struct {
	view   *variantC1
	world  *World
	oldest uint64
}

type Info struct {
	Config      *config.Config
	Content     string
	Running     bool
	StartErr    error
	Recorder    *telemetry.Recorder
	Simulations map[string]*simulation.Simulation
	Save        func(string) error
}

func New(info Info) *UI {
	if info.Config == nil {
		info.Config = &config.Config{}
	}
	w := newLiveWorld(info.Config, info.Simulations, info.Recorder)
	th := NewTheme()
	cfg := newConfigEditor(th, info)
	v := newVariantC1(th, w, cfg)
	return &UI{view: v, world: w}
}

func (u *UI) Layout(gtx C) D {
	paint.Fill(gtx.Ops, colCanvas)
	if u.world.lastPoll.IsZero() || gtx.Now.Sub(u.world.lastPoll) >= 100*time.Millisecond {
		u.world.Poll(gtx.Now)
	}
	if len(u.world.exchanges) > 0 {
		oldest := u.world.exchanges[u.world.head].Seq
		if oldest != u.oldest {
			u.oldest = oldest
			v := u.view.linked
			for seq := range v.rows {
				if seq < oldest {
					delete(v.rows, seq)
					delete(v.packetButtons, seq)
					delete(v.copyButtons, seq)
					delete(v.packets, seq)
				}
			}
		}
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(100 * time.Millisecond)})
	return u.view.Layout(gtx)
}
