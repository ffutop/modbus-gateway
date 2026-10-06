package workspace

import (
	"encoding/binary"
	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
	"time"
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
	Observed bool
	Online   bool
	Gateway  *Gateway
}

func (d *Downstream) RouteIDs() string {
	if d.SlaveIDs != "" {
		return d.SlaveIDs
	}
	return "默认路由（全部 ID）"
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
	case "rtu-over-tcp":
		return "RTU over TCP"
	case "rtu":
		return "Modbus RTU"
	}
	if d.InProcess() {
		return "进程内"
	}
	return d.Type
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
	if g.UpType == "tcp" {
		return "Modbus TCP"
	}
	return g.UpType
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
