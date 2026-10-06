package sidecar

import (
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestManagementConnectionLossDoesNotReportStaleListeners(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/status" {
			if fail.Load() {
				http.Error(w, "disconnected", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"upstreams":[{"gateway":"demo","index":0,"state":"listening"}]}`))
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, "", nil)
	defer client.Close()
	eventually(t, "management connected", func() bool { u := client.Upstreams(); return len(u) == 1 && u[0].State == gateway.UpstreamListening })
	fail.Store(true)
	eventually(t, "management lost", func() bool { return client.ConnectionError() != nil })
	if client.Upstreams() != nil {
		t.Fatal("stale listener states reported as current")
	}
	fail.Store(false)
	eventually(t, "management reconnected", func() bool { return client.ConnectionError() == nil && len(client.Upstreams()) == 1 })
}
