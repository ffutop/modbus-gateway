// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/internal/local-slave/persistence"
	"github.com/ffutop/modbus-gateway/internal/simulation"
	"github.com/ffutop/modbus-gateway/internal/telemetry"
	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/transport"
	"github.com/ffutop/modbus-gateway/transport/local"
	"github.com/ffutop/modbus-gateway/transport/tcp"
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
	})

	var got struct {
		Version     string `json:"version"`
		ConfigPath  string `json:"config_path"`
		Simulations []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Version uint64 `json:"version"`
		} `json:"simulations"`
	}
	getJSON(t, h, "/api/v1/status", &got)

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

func TestMetrics_CountsRequestsPerGatewayAndDownstream(t *testing.T) {
	sim := openSim(t, "line-a")
	rec := telemetry.NewRecorder(100)
	client := runGateway(t, sim, rec)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := client.Send(ctx, 100, readHolding0); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	client.Send(ctx, 42, readHolding0) // no route for slave 42

	var got struct {
		Gateways []struct {
			Name        string `json:"name"`
			Requests    uint64 `json:"requests"`
			Errors      uint64 `json:"errors"`
			Downstreams []struct {
				Name     string  `json:"name"`
				Requests uint64  `json:"requests"`
				Errors   uint64  `json:"errors"`
				P99Ms    float64 `json:"p99_ms"`
			} `json:"downstreams"`
		} `json:"gateways"`
	}
	getJSON(t, NewHandler(Deps{Telemetry: rec}), "/api/v1/metrics", &got)

	if len(got.Gateways) != 1 {
		t.Fatalf("gateways = %+v, want 1", got.Gateways)
	}
	g := got.Gateways[0]
	if g.Name != "business" || g.Requests != 6 || g.Errors != 1 {
		t.Errorf("gateway = %s requests=%d errors=%d, want business 6/1", g.Name, g.Requests, g.Errors)
	}
	if len(g.Downstreams) != 1 || g.Downstreams[0].Name != "plc-sim" || g.Downstreams[0].Requests != 5 || g.Downstreams[0].Errors != 0 {
		t.Errorf("downstreams = %+v, want [plc-sim 5/0]", g.Downstreams)
	}
	if len(g.Downstreams) == 1 && g.Downstreams[0].P99Ms <= 0 {
		t.Errorf("plc-sim p99 = %v ms, want the measured forwarding latency (> 0)", g.Downstreams[0].P99Ms)
	}
}

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

const sampleConfig = `# Field gateway, line A
version: 1

simulations:
  - name: line-a
    persistence: { type: memory }

gateways:
  - name: business
    upstreams:
      - type: tcp
        tcp: { address: "127.0.0.1:15020" }
    downstreams:
      - name: plc-sim # PLC under test
        type: local
        slave_ids: "100"
        simulation: { ref: line-a }
`

// writeConfig writes content to a temp config file and returns its path and
// the revision the process would have recorded when loading it at startup.
func writeConfig(t *testing.T, content string) (path, revision string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, config.Revision([]byte(content))
}

type configResponse struct {
	Revision       string `json:"revision"`
	SchemaVersion  int    `json:"schema_version"`
	RunningMatches bool   `json:"running_matches"`
	Config         struct {
		Gateways []struct {
			Name        string `json:"name"`
			Downstreams []struct {
				Name     string `json:"name"`
				SlaveIDs string `json:"slave_ids"`
			} `json:"downstreams"`
		} `json:"gateways"`
	} `json:"config"`
}

func TestConfig_ReturnsTheFileOnDiskAndWhetherItIsRunning(t *testing.T) {
	path, startup := writeConfig(t, sampleConfig)
	h := NewHandler(Deps{ConfigPath: path, StartupRevision: startup})

	var got configResponse
	getJSON(t, h, "/api/v1/config", &got)
	if got.Revision == "" || !got.RunningMatches || got.SchemaVersion != 1 {
		t.Errorf("revision=%q running_matches=%v schema=%d, want a revision, true, 1", got.Revision, got.RunningMatches, got.SchemaVersion)
	}
	if gw := got.Config.Gateways; len(gw) != 1 || gw[0].Downstreams[0].Name != "plc-sim" || gw[0].Downstreams[0].SlaveIDs != "100" {
		t.Errorf("config tree = %+v", got.Config)
	}

	// Someone edits the file in a text editor; the process is still running the old one.
	if err := os.WriteFile(path, []byte(strings.Replace(sampleConfig, `"100"`, `"101"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	var after configResponse
	getJSON(t, h, "/api/v1/config", &after)
	if after.RunningMatches || after.Revision == got.Revision || after.Config.Gateways[0].Downstreams[0].SlaveIDs != "101" {
		t.Errorf("after edit: revision=%q running_matches=%v slave_ids=%q", after.Revision, after.RunningMatches, after.Config.Gateways[0].Downstreams[0].SlaveIDs)
	}
}

const twoDownstreamConfig = `version: 1
simulations:
  - name: line-a
    persistence: { type: memory }
gateways:
  - name: business
    upstreams:
      - type: tcp
        tcp: { address: "127.0.0.1:15021" }
    downstreams:
      - name: plc-sim
        type: local
        slave_ids: "100"
        simulation: { ref: line-a }
      - name: boiler-sim
        type: local
        slave_ids: "101"
        simulation: { ref: line-a }
`

type problem struct {
	Path    []any  `json:"path"`
	Message string `json:"message"`
}

func postJSON(t *testing.T, h http.Handler, method, path string, body any, out any) int {
	t.Helper()
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewReader(b)))
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("%s %s: decode %q: %v", method, path, rec.Body.String(), err)
	}
	return rec.Code
}

func setEdit(value any, path ...any) map[string]any {
	return map[string]any{"op": "set", "path": path, "value": value}
}

func TestValidate_LocatesSlaveIDConflictWithoutWritingTheFile(t *testing.T) {
	path, rev := writeConfig(t, twoDownstreamConfig)
	h := NewHandler(Deps{ConfigPath: path, StartupRevision: rev})

	var got struct {
		Valid    bool      `json:"valid"`
		Problems []problem `json:"problems"`
	}
	code := postJSON(t, h, http.MethodPost, "/api/v1/config/validate", map[string]any{
		"base_revision": rev,
		"edits":         []any{setEdit("100", "gateways", 0, "downstreams", 1, "slave_ids")},
	}, &got)

	if code != http.StatusOK || got.Valid || len(got.Problems) != 1 {
		t.Fatalf("code=%d valid=%v problems=%+v, want 200, false, one problem", code, got.Valid, got.Problems)
	}
	p := got.Problems[0]
	if fmt.Sprint(p.Path) != "[gateways 0 downstreams 1 slave_ids]" || !strings.Contains(p.Message, "plc-sim") {
		t.Errorf("problem = %+v, want it on downstreams[1].slave_ids naming plc-sim", p)
	}
	if content, _ := os.ReadFile(path); string(content) != twoDownstreamConfig {
		t.Error("validate modified the config file")
	}

	code = postJSON(t, h, http.MethodPost, "/api/v1/config/validate", map[string]any{
		"base_revision": rev,
		"edits":         []any{setEdit("102", "gateways", 0, "downstreams", 1, "slave_ids")},
	}, &got)
	if code != http.StatusOK || !got.Valid || len(got.Problems) != 0 {
		t.Errorf("valid edit: code=%d valid=%v problems=%+v", code, got.Valid, got.Problems)
	}
}

// handWrittenConfig has the things people care about in their own files:
// comments, blank lines, flow maps with inner spaces, quoting choices.
const handWrittenConfig = `# Line A field gateway — owned by the controls team
version: 1

simulations:
  - name: line-a   # shared by plc-sim and boiler-sim
    persistence: { type: memory }

gateways:
  - name: business
    upstreams:
      - type: tcp
        tcp: { address: "127.0.0.1:15022" }

    downstreams:
      # PLC under test
      - name: plc-sim
        type: local
        slave_ids: "100"
        simulation: { ref: line-a }
      - name: boiler-sim
        type: local
        slave_ids: "101" # boiler controller
        simulation: { ref: line-a }
`

func TestSave_ChangesOnlyTheEditedValues(t *testing.T) {
	path, rev := writeConfig(t, handWrittenConfig)
	h := NewHandler(Deps{ConfigPath: path, StartupRevision: rev})

	var got struct {
		Revision        string `json:"revision"`
		RestartRequired bool   `json:"restart_required"`
	}
	code := postJSON(t, h, http.MethodPut, "/api/v1/config", map[string]any{
		"base_revision": rev,
		"edits": []any{
			setEdit("102", "gateways", 0, "downstreams", 1, "slave_ids"),
			setEdit("boiler-2", "gateways", 0, "downstreams", 1, "name"),
		},
	}, &got)
	if code != http.StatusOK || !got.RestartRequired {
		t.Fatalf("code=%d restart_required=%v", code, got.RestartRequired)
	}

	want := strings.NewReplacer(`slave_ids: "101"`, `slave_ids: "102"`, `name: boiler-sim`, `name: boiler-2`).Replace(handWrittenConfig)
	content, _ := os.ReadFile(path)
	if string(content) != want {
		t.Errorf("saved file differs beyond the two edits:\n--- got\n%s\n--- want\n%s", content, want)
	}
	if got.Revision != config.Revision(content) {
		t.Errorf("revision = %q, want the saved content's revision", got.Revision)
	}
}

func TestSave_RejectsInvalidDraftAndKeepsTheFile(t *testing.T) {
	path, rev := writeConfig(t, handWrittenConfig)
	h := NewHandler(Deps{ConfigPath: path, StartupRevision: rev})

	var got struct {
		Problems []problem `json:"problems"`
	}
	code := postJSON(t, h, http.MethodPut, "/api/v1/config", map[string]any{
		"base_revision": rev,
		"edits":         []any{setEdit("100", "gateways", 0, "downstreams", 1, "slave_ids")},
	}, &got)
	if code != http.StatusUnprocessableEntity || len(got.Problems) == 0 {
		t.Errorf("code=%d problems=%+v, want 422 with problems", code, got.Problems)
	}
	if content, _ := os.ReadFile(path); string(content) != handWrittenConfig {
		t.Error("an invalid draft was written to disk")
	}
}

func TestSave_RefusesWhenTheFileChangedSinceItWasRead(t *testing.T) {
	path, rev := writeConfig(t, handWrittenConfig)
	h := NewHandler(Deps{ConfigPath: path, StartupRevision: rev})

	// Someone saves from a text editor after the console read the file.
	edited := strings.Replace(handWrittenConfig, "# PLC under test", "# PLC under test (rack 2)", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Error           string `json:"error"`
		CurrentRevision string `json:"current_revision"`
	}
	code := postJSON(t, h, http.MethodPut, "/api/v1/config", map[string]any{
		"base_revision": rev,
		"edits":         []any{setEdit("102", "gateways", 0, "downstreams", 1, "slave_ids")},
	}, &got)
	if code != http.StatusConflict || got.CurrentRevision != config.Revision([]byte(edited)) {
		t.Errorf("code=%d current_revision=%q, want 409 with the file's current revision", code, got.CurrentRevision)
	}
	if content, _ := os.ReadFile(path); string(content) != edited {
		t.Error("the text editor's change was overwritten")
	}
}

func TestSave_RefusesLegacyV0Configs(t *testing.T) {
	const v0 = `gateways:
  - name: legacy
    upstreams: [{ type: tcp, tcp: { address: "127.0.0.1:15023" } }]
    downstreams:
      - type: local
        slave_ids: "100"
`
	path, rev := writeConfig(t, v0)
	h := NewHandler(Deps{ConfigPath: path, StartupRevision: rev})

	var got struct {
		Error string `json:"error"`
	}
	code := postJSON(t, h, http.MethodPut, "/api/v1/config", map[string]any{
		"base_revision": rev,
		"edits":         []any{setEdit("101", "gateways", 0, "downstreams", 0, "slave_ids")},
	}, &got)
	if code != http.StatusUnprocessableEntity || !strings.Contains(got.Error, "version: 1") {
		t.Errorf("code=%d error=%q, want 422 asking to migrate to version: 1", code, got.Error)
	}
	if content, _ := os.ReadFile(path); string(content) != v0 {
		t.Error("v0 config was modified")
	}
}

func TestHandler_WithoutTelemetryServesEmptyMetricsInsteadOfPanicking(t *testing.T) {
	h := NewHandler(Deps{})
	var got struct {
		Gateways []any `json:"gateways"`
	}
	getJSON(t, h, "/api/v1/metrics", &got)
	if got.Gateways == nil || len(got.Gateways) != 0 {
		t.Errorf("gateways = %v, want an empty list", got.Gateways)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("events: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestHandler_SetsHeadersThatStopFramingAndSniffing(t *testing.T) {
	h := NewHandler(Deps{Static: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}})
	for _, path := range []string{"/", "/api/v1/status"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		hdr := rec.Header()
		if hdr.Get("X-Frame-Options") != "DENY" || hdr.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: X-Frame-Options=%q X-Content-Type-Options=%q", path, hdr.Get("X-Frame-Options"), hdr.Get("X-Content-Type-Options"))
		}
		csp := hdr.Get("Content-Security-Policy")
		for _, want := range []string{"frame-ancestors 'none'", "script-src 'self'", "default-src 'self'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP %q lacks %q", path, csp, want)
			}
		}
	}
}

func TestRunningConfigPreservesStartupRoutesAfterDiskChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\ngateways: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := &config.Config{Gateways: []config.GatewayConfig{{Name: "running", Downstreams: []config.DownstreamConfig{{Name: "original", Type: "local", SlaveIDs: "100", SimulationRef: "model"}}}}, Simulations: []config.SimulationConfig{{Name: "model"}}}
	h := NewHandler(Deps{ConfigPath: path, RunningConfig: c})
	var tree map[string]any
	getJSON(t, h, "/api/v1/running-config", &tree)
	g := tree["gateways"].([]any)[0].(map[string]any)
	if g["name"] != "running" || g["downstreams"].([]any)[0].(map[string]any)["name"] != "original" {
		t.Fatalf("unexpected running tree: %v", tree)
	}
}

func TestRunningIndexMatchesConsoleEditsButNotExternalWrites(t *testing.T) {
	path, revision := writeConfig(t, sampleConfig)
	h := NewHandler(Deps{ConfigPath: path, StartupRevision: revision})
	var response struct {
		Revision     string `json:"revision"`
		IndexMatches bool   `json:"running_index_matches"`
	}
	getJSON(t, h, "/api/v1/config", &response)
	if !response.IndexMatches {
		t.Fatal("startup document should retain runtime indexes")
	}
	code := postJSON(t, h, http.MethodPut, "/api/v1/config", map[string]any{
		"base_revision": revision,
		"edits":         []any{setEdit("renamed", "gateways", 0, "downstreams", 0, "name")},
	}, &response)
	if code != http.StatusOK {
		t.Fatalf("save returned %d", code)
	}
	getJSON(t, h, "/api/v1/config", &response)
	if !response.IndexMatches {
		t.Fatal("renaming an existing scalar must retain runtime indexes")
	}
	if err := os.WriteFile(path, []byte(sampleConfig+"# external write\n"), 0600); err != nil {
		t.Fatal(err)
	}
	getJSON(t, h, "/api/v1/config", &response)
	if response.IndexMatches {
		t.Fatal("external versions must use name matching instead of positional matching")
	}
}
