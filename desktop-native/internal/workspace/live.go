package workspace

import (
	"fmt"
	"slices"
	"time"

	"gioui.org/op"
	"gioui.org/op/paint"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/desktop-native/internal/runlog"
	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/routing"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
)

const (
	maxExchanges = 5000                   // requests the workspace retains, oldest dropped first
	maxMasters   = 64                     // distinct upstream sources tracked per gateway
	pollInterval = 100 * time.Millisecond // while requests arrive
	idleRefresh  = time.Second            // otherwise
)

// World is a UI-owned projection of the running configuration and the live
// source. Poll and Layout run on the window goroutine.
type World struct {
	Gateways     []*Gateway
	Sims         []*Sim
	src          live.Source // nil shows no traffic
	rt           live.Runtime
	exchanges    []Exchange
	head         int // next overwritten slot once history reaches capacity
	cursor       uint64
	missed       uint64
	gen          uint64 // changes whenever the history changes
	lastPoll     time.Time
	lastTraffic  time.Time // the last poll that brought requests
	counts       map[Link][2]uint64
	rates        map[Link]float64
	observations map[Link]*observation
}

type observation struct {
	values [4][tableSize]uint16
	seen   [4][tableSize]time.Time
}

func newLiveWorld(cfg *config.Config, src live.Source) *World {
	w := &World{src: src, counts: map[Link][2]uint64{}, rates: map[Link]float64{}, observations: map[Link]*observation{}}
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
	if w.src == nil {
		w.lastPoll = now
		return
	}
	for _, e := range w.src.Since(w.cursor) {
		if e.Seq <= w.cursor {
			continue
		}
		if e.Seq > w.cursor+1 {
			w.missed += e.Seq - w.cursor - 1
		}
		w.cursor = e.Seq
		w.record(e)
		w.lastTraffic = now
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
	w.gen++
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
		if !found && len(g.Masters) < maxMasters {
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
	var buf [6]Link // stays on the stack: record runs once per request
	keys := append(buf[:0], Link{Gw: g}, Link{Gw: g, Ds: x.Ds})
	for _, m := range g.Masters {
		if m == e.Source {
			keys = append(keys, Link{Gw: g, Master: m}, Link{Gw: g, Master: m, Ds: x.Ds})
			break
		}
	}
	if x.Ds != nil && x.Ds.Sim != "" {
		keys = append(keys, Link{Sim: x.Ds.Sim}, Link{Gw: g, Sim: x.Ds.Sim})
	}
	for i, k := range keys {
		// Count the empty-downstream gateway key just once.
		if slices.Contains(keys[:i], k) {
			continue
		}
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

// Each calls fn on the history, oldest first, without copying it. The
// pointers stay valid until the history changes (see gen).
func (w *World) Each(fn func(*Exchange)) {
	for i := range w.exchanges {
		fn(&w.exchanges[(w.head+i)%len(w.exchanges)])
	}
}

// active reports whether requests arrived recently, so the view should keep
// refreshing rates and rows at full pace.
func (w *World) active(now time.Time) bool {
	return !w.lastTraffic.IsZero() && now.Sub(w.lastTraffic) < 2*time.Second
}

// Exchanges returns a copy of the matching history, oldest first.
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

// liveTables maps the workspace's tables to the source's.
var liveTables = [...]live.Table{holding: live.Holding, input: live.Input, coils: live.Coils, discrete: live.Discrete}

// Snapshot returns a model's current values; ok is false when the model is
// unavailable. The model does not expose per-address write times.
func (w *World) Snapshot(name string, t table) (values [tableSize]uint16, changed [tableSize]time.Time, ok bool) {
	if w.src == nil {
		return
	}
	got, ok := w.src.Registers(name, liveTables[t], 0, tableSize)
	copy(values[:], got)
	return values, changed, ok && len(got) == tableSize
}

// UI is the production workspace. It never generates traffic.
type UI struct {
	view   *variantC1
	world  *World
	oldest uint64
}

type Info struct {
	Logs           runlog.Source
	Notify         func()
	RunningContent string
	DraftBase      string
	Draft          string
	DraftConflict  bool
	SaveDraft      func(string)
	ClearDraft     func() error
	DraftError     func() error
	Config         *config.Config
	Content        string // the config file text the gateway was started with
	Running        bool   // the gateway was started (or is starting) on Content
	StartErr       error
	Source         live.Source  // nil shows no traffic
	Runtime        live.Runtime // nil: a fixed state from Running and StartErr
	Rebase         func(string) error
	Save           func(string) error
	NewFile        bool                          // Config.Path does not exist yet; the first save creates it
	Notice         string                        // shown once, e.g. after another file was opened
	Open           func(path string) error       // switch to another configuration file
	SaveAs         func(path, text string) error // write the draft elsewhere and switch to it
	// PickFile shows the platform's file chooser and returns "" if cancelled;
	// nil, or filepicker.ErrUnavailable, falls back to the in-app dialog.
	PickFile func(save bool, dir, name string) (string, error)
}

func New(info Info) *UI {
	if info.Config == nil {
		info.Config = &config.Config{}
	}
	if info.Runtime == nil {
		st := live.State{Phase: live.Running}
		if !info.Running {
			st = live.State{Phase: live.Stopped, Err: info.StartErr}
		}
		info.Runtime = live.Fixed(st)
	}
	running := info.Config
	if info.RunningContent != "" {
		if cfg, err := config.ParseDraft([]byte(info.RunningContent)); err == nil {
			running = cfg
		}
	}
	w := newLiveWorld(running, info.Source)
	w.rt = info.Runtime
	th := NewTheme()
	cfg := newConfigEditor(th, info)
	v := newVariantC1(th, w, cfg)
	v.shell.logs = newLogView(th, info.Logs, info.Notify)
	return &UI{view: v, world: w}
}

func (u *UI) Layout(gtx C) D {
	paint.Fill(gtx.Ops, colCanvas)
	if u.world.lastPoll.IsZero() || gtx.Now.Sub(u.world.lastPoll) >= pollInterval {
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
	// Poll at full pace while traffic flows; when idle, a slow heartbeat keeps
	// ages current, and the source wakes the window when data arrives.
	next := idleRefresh
	if u.world.active(gtx.Now) {
		next = pollInterval
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(next)})
	return u.view.Layout(gtx)
}

// CarryViewTo keeps the page shown after another file was opened.
func (u *UI) CarryViewTo(next *UI) {
	next.view.shell.module = u.view.shell.module
	next.view.shell.logs = u.view.shell.logs
}

// CarryDraftTo keeps edits made while a restart was in flight reviewable after
// the workspace is reconstructed. It never changes the saved or running text.
func (u *UI) CarryDraftTo(next *UI) {
	u.CarryViewTo(next)
	if u.view.shell.cfg.unsaved() {
		next.view.shell.cfg.recoveryText = u.view.shell.cfg.recoveryContent()
		next.view.shell.cfg.recoveryConflict = false
		next.view.shell.module = 1
	}
}
