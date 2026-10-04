// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package sidecar

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ffutop/modbus-gateway/desktop-native/internal/live"
	"github.com/ffutop/modbus-gateway/internal/cli"
	"github.com/ffutop/modbus-gateway/internal/gateway"
	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/transport/tcp"
)

// The test binary doubles as the gateway child, as the app binary does.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == Flag {
		cli.Main("test", os.Args[2:])
		return
	}
	os.Exit(m.Run())
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func writeConfig(t *testing.T, upstreams ...string) string {
	t.Helper()
	var ups strings.Builder
	for _, a := range upstreams {
		fmt.Fprintf(&ups, "      - type: tcp\n        tcp: {address: %q}\n", a)
	}
	text := fmt.Sprintf(`version: 1
simulations:
  - name: model
    persistence: {type: memory}
gateways:
  - name: demo
    upstreams:
%s    downstreams:
      - name: local
        type: local
        slave_ids: "1"
        simulation: {ref: model}
`, ups.String())
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// notified counts the supervisor's repaint requests.
var notified atomic.Int64

func newSupervisor(t *testing.T, config string) *Supervisor {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := NewSupervisor(exe, config, func() { notified.Add(1) })
	t.Cleanup(s.Stop)
	return s
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func phase(s *Supervisor, p live.Phase) func() bool {
	return func() bool { return s.State().Phase == p }
}

func TestSidecarServesTrafficRegistersAndListenerState(t *testing.T) {
	up := freeAddr(t)
	s := newSupervisor(t, writeConfig(t, up))
	s.Start()
	eventually(t, "running", phase(s, live.Running))
	if s.State().Epoch != 0 {
		t.Fatalf("a successful first start changed the epoch: %+v", s.State())
	}
	eventually(t, "listener state", func() bool {
		u := s.Upstreams()
		return len(u) == 1 && u[0].State == gateway.UpstreamListening
	})

	ctx := context.Background()
	client := tcp.NewClient(up)
	if err := client.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Send(ctx, 1, modbus.ProtocolDataUnit{FunctionCode: 6, Data: []byte{0, 3, 0, 42}}); err != nil {
		t.Fatal(err)
	}
	before := notified.Load()
	eventually(t, "the write event", func() bool { return len(s.Since(0)) == 1 })
	eventually(t, "a repaint request for the new request", func() bool { return notified.Load() > before })
	e := s.Since(0)[0]
	if e.Gateway != "demo" || e.Downstream != "local" || e.SlaveID != 1 || e.Err != nil ||
		fmt.Sprintf("% x", e.Request) != "06 00 03 00 2a" || fmt.Sprintf("% x", e.Response) != "06 00 03 00 2a" || e.Time.IsZero() {
		t.Fatalf("event = %+v", e)
	}
	if len(s.Since(e.Seq)) != 0 {
		t.Fatal("Since returned an event already read")
	}
	eventually(t, "the written register", func() bool {
		v, ok := s.Registers("model", live.Holding, 0, 8)
		return ok && len(v) == 8 && v[3] == 42
	})
	if _, ok := s.Registers("missing", live.Holding, 0, 8); ok {
		t.Fatal("an unknown model reported values")
	}
}

func TestRestartReloadsTheConfigAndChangesTheEpoch(t *testing.T) {
	first, second := freeAddr(t), freeAddr(t)
	config := writeConfig(t, first)
	s := newSupervisor(t, config)
	s.Start()
	eventually(t, "running", phase(s, live.Running))

	// The saved file now names a second listener; a restart applies it.
	if err := os.WriteFile(config, []byte(strings.Replace(mustRead(t, config), first, second, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	s.Restart()
	if p := s.State().Phase; p != live.Starting {
		t.Fatalf("phase right after Restart = %v, want Starting", p)
	}
	eventually(t, "restarted", func() bool { st := s.State(); return st.Phase == live.Running && st.Epoch == 1 })
	if _, err := net.Dial("tcp", second); err != nil {
		t.Fatalf("new listener not up after restart: %v", err)
	}
	if c, err := net.Dial("tcp", first); err == nil {
		c.Close()
		t.Fatal("old listener still accepting after restart")
	}
}

func TestStartFailureExplainsWithTheGatewayOutput(t *testing.T) {
	s := newSupervisor(t, filepath.Join(t.TempDir(), "missing.yaml"))
	s.Start()
	eventually(t, "stopped", phase(s, live.Stopped))
	st := s.State()
	if st.Err == nil || !strings.Contains(st.Err.Error(), "网关启动失败") || !strings.Contains(st.Err.Error(), "Failed to load configuration") || st.Epoch != 1 {
		t.Fatalf("state = %+v, want a start failure quoting the gateway output, epoch 1", st)
	}
	if _, ok := s.Registers("model", live.Holding, 0, 1); ok {
		t.Fatal("a stopped gateway reported register values")
	}
}

func TestListenerFailureIsReportedWhileRunning(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	s := newSupervisor(t, writeConfig(t, freeAddr(t), busy.Addr().String()))
	s.Start()
	eventually(t, "one listener failed", func() bool {
		u := s.Upstreams()
		return len(u) == 2 && u[0].State == gateway.UpstreamListening && u[1].State == gateway.UpstreamFailed && u[1].Error != ""
	})
}

func TestCrashIsReportedAndRestartRecovers(t *testing.T) {
	s := newSupervisor(t, writeConfig(t, freeAddr(t)))
	s.Start()
	eventually(t, "running", phase(s, live.Running))
	s.mu.Lock()
	p := s.proc
	s.mu.Unlock()
	p.cmd.Process.Kill()
	eventually(t, "crash noticed", phase(s, live.Stopped))
	if st := s.State(); st.Err == nil || !strings.Contains(st.Err.Error(), "网关意外退出") || st.Epoch != 0 {
		t.Fatalf("state = %+v, want an unexpected exit, same epoch", st)
	}
	s.Restart()
	eventually(t, "recovered", func() bool { st := s.State(); return st.Phase == live.Running && st.Epoch == 1 })
}

func TestStopEndsTheChildGracefully(t *testing.T) {
	s := newSupervisor(t, writeConfig(t, freeAddr(t)))
	s.Start()
	eventually(t, "running", phase(s, live.Running))
	s.mu.Lock()
	p := s.proc
	s.mu.Unlock()
	s.Stop()
	select {
	case <-p.Done():
	default:
		t.Fatal("Stop returned before the child exited")
	}
	if p.Err() != nil {
		t.Fatalf("child exit = %v, want a clean exit after stdin closed", p.Err())
	}
	if tail := strings.Join(s.Output().Tail(outputLines), "\n"); !strings.Contains(tail, "stdin closed") {
		t.Fatalf("output lacks the graceful shutdown log:\n%s", tail)
	}
	s.Restart()
	if s.State().Phase != live.Stopped {
		t.Fatal("Restart after Stop started the gateway again")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
