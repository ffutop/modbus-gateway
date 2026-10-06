package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func scaleConfiguration() string {
	var b strings.Builder
	b.WriteString("version: 1\nsimulations:\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "  - name: 模拟模型-%d\n    persistence: {type: memory}\n", i)
	}
	b.WriteString("gateways:\n")
	for g := 0; g < 100; g++ {
		fmt.Fprintf(&b, "  - name: 网关-%d\n    upstreams: [{type: tcp, tcp: {address: '127.0.0.1:%d'}}]\n    downstreams:\n", g, 20000+g)
		for d := 0; d < 10; d++ {
			idx := g*10 + d
			fmt.Fprintf(&b, "      - name: 下游设备-%d\n        type: injector\n        slave_ids: '%d'\n        simulation:\n          ref: 模拟模型-%d\n          mappings:\n", idx, d+1, idx%200)
			for m := 0; m < 2; m++ {
				fmt.Fprintf(&b, "            - source: {table: holding_registers, start_address: %d, count: 4}\n              target: {table: input_registers, start_address: %d}\n", m*4, idx*8+m*4)
			}
		}
	}
	return b.String()
}

// Opt-in timing evidence uses real workspace layout, not a parallel search implementation.
func TestConfigurationScaleInteraction(t *testing.T) {
	path := os.Getenv("WORKSPACE_SCALE_REPORT")
	if path == "" {
		t.Skip("WORKSPACE_SCALE_REPORT not set")
	}
	started := time.Now()
	e := structureEditor(t, scaleConfiguration())
	if ps := e.draft.Problems(); len(ps) > 0 {
		t.Fatal(ps[0])
	}
	load := time.Since(started)
	e.saveDraft = func(string) {}
	wbFrame(e)
	samples := []float64{}
	for i := 0; i < 40; i++ {
		query := []string{"下游设备-1", "模拟模型-8", "网关-9", "holding_registers", "不存在"}[i%5]
		began := time.Now()
		e.wb.query.SetText(query)
		wbFrame(e)
		samples = append(samples, float64(time.Since(began).Microseconds())/1000)
	}
	sort.Float64s(samples)
	p95 := samples[(len(samples)*95+99)/100-1]
	report := map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version(), "cpus": runtime.NumCPU(), "gateways": 100, "downstreams": 1000, "models": 200, "mappings": 2000, "load_ms": float64(load.Microseconds()) / 1000, "search_layout_p95_ms": p95, "search_layout_max_ms": samples[len(samples)-1], "samples_ms": samples, "window": "1100x680", "threshold_ms": 200}
	bytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("search/layout p95 %.2f ms; load %.2f ms", p95, float64(load.Microseconds())/1000)
	if p95 > 200 {
		t.Fatalf("search/layout p95 %.2f ms exceeds 200 ms", p95)
	}
}
