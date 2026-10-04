// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/internal/modbus"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/internal/simulation/persistence"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
	"github.com/ffutop/modbus-gateway/internal/transport"
	"github.com/ffutop/modbus-gateway/internal/transport/local"
	"github.com/ffutop/modbus-gateway/internal/transport/tcp"
)

func openSim(t *testing.T, name string) *simulation.Simulation {
	t.Helper()
	sim, err := simulation.Open(name, persistence.NewMemoryStorage())
	if err != nil {
		t.Fatalf("open simulation %q: %v", name, err)
	}
	return sim
}

func getJSON(t *testing.T, h http.Handler, path string, out any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d, body %s", path, rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("GET %s: decode: %v (body %s)", path, err, rec.Body.String())
	}
}

func TestStatus_ReportsVersionConfigPathAndSimulations(t *testing.T) {
	sim := openSim(t, "line-a")
	if err := sim.WriteSingleRegister(0, 42); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Deps{
		Version:     "0.6.0-test",
		ConfigPath:  "/etc/modbusgw/config.yaml",
		Simulations: []*simulation.Simulation{sim},
		Upstreams: func() []gateway.UpstreamStatus {
			return []gateway.UpstreamStatus{{Gateway: "business", Index: 1, State: gateway.UpstreamFailed, Error: "address in use"}}
		},
	})

	var got struct {
		Version     string `json:"version"`
		ConfigPath  string `json:"config_path"`
		Simulations []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Version uint64 `json:"version"`
		} `json:"simulations"`
		Upstreams []gateway.UpstreamStatus `json:"upstreams"`
	}
	getJSON(t, h, "/api/v1/status", &got)
	if want := (gateway.UpstreamStatus{Gateway: "business", Index: 1, State: gateway.UpstreamFailed, Error: "address in use"}); len(got.Upstreams) != 1 || got.Upstreams[0] != want {
		t.Errorf("upstreams = %+v, want [%+v]", got.Upstreams, want)
	}

	if got.Version != "0.6.0-test" || got.ConfigPath != "/etc/modbusgw/config.yaml" {
		t.Errorf("process info = %q, %q", got.Version, got.ConfigPath)
	}
	if len(got.Simulations) != 1 {
		t.Fatalf("simulations = %+v, want 1 entry", got.Simulations)
	}
	s := got.Simulations[0]
	if s.Name != "line-a" || s.Status != "ready" || s.Version != 1 {
		t.Errorf("simulation = %+v, want line-a/ready/version 1", s)
	}
}

// runGateway starts a real gateway named "business" with one TCP upstream and
// a local downstream "plc-sim" on slave 100, recording into rec. It returns a
// Modbus TCP client pointed at the upstream.
func runGateway(t *testing.T, sim *simulation.Simulation, rec *telemetry.Recorder) *tcp.Client {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	plc := local.NewClient(sim)
	gw := gateway.NewGateway("business", []transport.Upstream{tcp.NewServer(addr)}, map[byte]transport.Downstream{100: plc}, nil)
	gw.Telemetry = rec
	gw.DownstreamNames = map[transport.Downstream]string{plc: "plc-sim"}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { gw.Start(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("gateway upstream %s never came up", addr)
		}
	}
	client := tcp.NewClient(addr)
	t.Cleanup(func() { client.Close() })
	return client
}

var readHolding0 = modbus.ProtocolDataUnit{FunctionCode: 3, Data: []byte{0, 0, 0, 1}}

// sseEvent is the shape of one request in an /api/v1/events batch.
type sseEvent struct {
	Seq          uint64  `json:"seq"`
	Time         string  `json:"time"`
	Gateway      string  `json:"gateway"`
	Downstream   string  `json:"downstream"`
	Source       string  `json:"source"`
	SlaveID      int     `json:"slave_id"`
	FunctionCode int     `json:"function_code"`
	Address      int     `json:"address"`
	Quantity     int     `json:"quantity"`
	DurationMs   float64 `json:"duration_ms"`
	Error        string  `json:"error"`
	Request      string  `json:"request"`
	Response     string  `json:"response"`
}

// readSSEBatches reads `data:` lines from an SSE response until want events
// have arrived or the deadline passes.
func readSSEBatches(t *testing.T, url string, want int) []sseEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	var got []sseEvent
	sc := bufio.NewScanner(resp.Body)
	for len(got) < want && sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var batch []sseEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &batch); err != nil {
			t.Fatalf("decode batch %q: %v", line, err)
		}
		got = append(got, batch...)
	}
	if len(got) < want {
		t.Fatalf("got %d events before deadline, want %d: %+v", len(got), want, got)
	}
	return got
}

func TestEvents_StreamsEachRequestWithRouteAndRange(t *testing.T) {
	sim := openSim(t, "line-a")
	rec := telemetry.NewRecorder(100)
	client := runGateway(t, sim, rec)
	srv := httptest.NewServer(NewHandler(Deps{Telemetry: rec}))
	defer srv.Close()
	ctx := context.Background()

	// One request before subscribing (delivered as backlog), two after.
	client.Send(ctx, 100, readHolding0)
	go func() {
		time.Sleep(300 * time.Millisecond)
		client.Send(ctx, 100, modbus.ProtocolDataUnit{FunctionCode: 16, Data: []byte{0, 10, 0, 2, 4, 0, 1, 0, 2}})
		client.Send(ctx, 42, readHolding0)
	}()

	got := readSSEBatches(t, srv.URL+"/api/v1/events", 3)

	first, write, noRoute := got[0], got[1], got[2]
	if first.Gateway != "business" || first.Downstream != "plc-sim" || first.SlaveID != 100 ||
		first.FunctionCode != 3 || first.Address != 0 || first.Quantity != 1 || first.Error != "" {
		t.Errorf("read event = %+v", first)
	}
	if first.Source == "" || first.Time == "" {
		t.Errorf("read event lacks source/time: %+v", first)
	}
	if write.FunctionCode != 16 || write.Address != 10 || write.Quantity != 2 {
		t.Errorf("write event = %+v, want fc16 address 10 quantity 2", write)
	}
	if noRoute.SlaveID != 42 || noRoute.Downstream != "" || noRoute.Error == "" {
		t.Errorf("no-route event = %+v, want slave 42 with an error and no downstream", noRoute)
	}
	if first.Request != "0300000001" || first.Response != "03020000" {
		t.Errorf("read PDUs = %q / %q, want 0300000001 / 03020000", first.Request, first.Response)
	}
	if noRoute.Request != "0300000001" || noRoute.Response != "" {
		t.Errorf("no-route PDUs = %q / %q, want the request and no response", noRoute.Request, noRoute.Response)
	}
	if !(first.Seq < write.Seq && write.Seq < noRoute.Seq) {
		t.Errorf("seq not increasing: %d %d %d", first.Seq, write.Seq, noRoute.Seq)
	}
}

func TestRegisters_MatchWhatModbusClientsWrote(t *testing.T) {
	sim := openSim(t, "line-a")
	client := runGateway(t, sim, telemetry.NewRecorder(10))
	ctx := context.Background()
	if _, err := client.Send(ctx, 100, modbus.ProtocolDataUnit{FunctionCode: 16, Data: []byte{0, 10, 0, 2, 4, 0x04, 0xD2, 0x16, 0x2E}}); err != nil {
		t.Fatal(err) // holding[10]=1234, holding[11]=5678
	}
	if _, err := client.Send(ctx, 100, modbus.ProtocolDataUnit{FunctionCode: 5, Data: []byte{0, 3, 0xFF, 0}}); err != nil {
		t.Fatal(err) // coil[3]=on
	}
	h := NewHandler(Deps{Simulations: []*simulation.Simulation{sim}})

	var regs struct {
		Table  string   `json:"table"`
		Start  int      `json:"start"`
		Values []uint16 `json:"values"`
	}
	getJSON(t, h, "/api/v1/simulations/line-a/registers?table=holding_registers&start=10&count=2", &regs)
	if regs.Table != "holding_registers" || regs.Start != 10 || len(regs.Values) != 2 || regs.Values[0] != 1234 || regs.Values[1] != 5678 {
		t.Errorf("holding registers = %+v, want start 10 values [1234 5678]", regs)
	}

	getJSON(t, h, "/api/v1/simulations/line-a/registers?table=coils&start=0&count=8", &regs)
	if want := []uint16{0, 0, 0, 1, 0, 0, 0, 0}; fmt.Sprint(regs.Values) != fmt.Sprint(want) {
		t.Errorf("coils = %v, want %v", regs.Values, want)
	}
}

func TestRegisters_RejectsBadRequests(t *testing.T) {
	h := NewHandler(Deps{Simulations: []*simulation.Simulation{openSim(t, "line-a")}})
	for path, want := range map[string]int{
		"/api/v1/simulations/nope/registers?table=coils&start=0&count=1":             http.StatusNotFound,
		"/api/v1/simulations/line-a/registers?table=bogus&start=0&count=1":           http.StatusBadRequest,
		"/api/v1/simulations/line-a/registers?table=coils&start=0&count=0":           http.StatusBadRequest,
		"/api/v1/simulations/line-a/registers?table=coils&start=0&count=4096":        http.StatusBadRequest,
		"/api/v1/simulations/line-a/registers?table=coils&start=65535&count=2":       http.StatusBadRequest,
		"/api/v1/simulations/line-a/registers?table=input_registers&start=x&count=1": http.StatusBadRequest,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d (body %s)", path, rec.Code, want, rec.Body.String())
		}
	}
}

func TestHandler_SetsHeadersThatStopFramingAndSniffing(t *testing.T) {
	h := NewHandler(Deps{})
	for _, path := range []string{"/", "/api/v1/status"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		hdr := rec.Header()
		if hdr.Get("X-Frame-Options") != "DENY" || hdr.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: X-Frame-Options=%q X-Content-Type-Options=%q", path, hdr.Get("X-Frame-Options"), hdr.Get("X-Content-Type-Options"))
		}
		csp := hdr.Get("Content-Security-Policy")
		for _, want := range []string{"frame-ancestors 'none'", "default-src 'none'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP %q lacks %q", path, csp, want)
			}
		}
	}
}
