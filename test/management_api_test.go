package test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// managementConfig is a minimal v1 config with one local simulation; uiBlock
// is spliced in verbatim (empty means no `ui` section at all).
func managementConfig(modbusPort int, uiBlock string) string {
	return fmt.Sprintf(`
version: 1
%s
simulations:
  - name: sim-device
    persistence: { type: memory }

gateways:
  - name: business-modbus
    upstreams:
      - type: tcp
        tcp: { address: "127.0.0.1:%d" }
    downstreams:
      - name: plc-sim
        type: local
        slave_ids: "100"
        simulation: { ref: sim-device }
`, uiBlock, modbusPort)
}

func TestManagementAPI_ServesStatusWhenUIEnabled(t *testing.T) {
	uiPort := freePort(t)
	stop := startGatewayWithConfig(t, "mgmt-enabled", managementConfig(freePort(t), fmt.Sprintf(`
ui:
  enabled: true
  listen: "127.0.0.1:%d"
`, uiPort)))
	defer stop()

	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", uiPort))
	if err != nil {
		t.Fatalf("GET /api/v1/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d", resp.StatusCode)
	}

	var got struct {
		Simulations []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"simulations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Simulations) != 1 || got.Simulations[0].Name != "sim-device" || got.Simulations[0].Status != "ready" {
		t.Errorf("simulations = %+v, want [sim-device ready]", got.Simulations)
	}
}

func portAcceptsConnections(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func TestManagementAPI_OpensNoPortUnlessEnabled(t *testing.T) {
	t.Run("explicitly disabled", func(t *testing.T) {
		addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
		stop := startGatewayWithConfig(t, "mgmt-disabled", managementConfig(freePort(t), fmt.Sprintf(`
ui:
  enabled: false
  listen: %q
`, addr)))
		defer stop()

		if portAcceptsConnections(addr) {
			t.Errorf("%s accepts connections although ui.enabled is false", addr)
		}
	})

	t.Run("no ui section", func(t *testing.T) {
		const defaultAddr = "127.0.0.1:8090"
		if portAcceptsConnections(defaultAddr) {
			t.Skipf("%s is already in use on this machine", defaultAddr)
		}
		stop := startGatewayWithConfig(t, "mgmt-absent", managementConfig(freePort(t), ""))
		defer stop()

		if portAcceptsConnections(defaultAddr) {
			t.Errorf("default management address %s opened without a ui section", defaultAddr)
		}
	})
}

// sidecar is the gateway binary started in sidecar mode, the way a parent
// process (such as the native desktop app) runs it.
type sidecar struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	addr  string // from the ui_ready line
	lines chan string
}

// startSidecar starts the binary with extra args/env and waits for its
// ui_ready line on stdout.
func startSidecar(t *testing.T, configContent string, env []string, args ...string) *sidecar {
	t.Helper()
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(gatewayBinaryPath, append([]string{"-config", configFile}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stderr = os.Stderr
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	sc := &sidecar{cmd: cmd, stdin: stdin, lines: make(chan string, 100)}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})

	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			var msg struct {
				Event string `json:"event"`
				Addr  string `json:"addr"`
			}
			if json.Unmarshal([]byte(line), &msg) == nil && msg.Event == "ui_ready" {
				ready <- msg.Addr
			}
			select {
			case sc.lines <- line:
			default:
			}
		}
		close(sc.lines)
	}()
	select {
	case sc.addr = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("no ui_ready line within 5s")
	}
	return sc
}

func (sc *sidecar) get(t *testing.T, path string, header map[string]string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "http://"+sc.addr+path, nil)
	for k, v := range header {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestSidecar_ReportsAddressAndRequiresToken(t *testing.T) {
	const token = "s3cret-token"
	sc := startSidecar(t, managementConfig(freePort(t), ""), []string{"MODMUX_UI_TOKEN=" + token}, "-ui-listen", "127.0.0.1:0")

	if !strings.HasPrefix(sc.addr, "127.0.0.1:") || strings.HasSuffix(sc.addr, ":0") {
		t.Fatalf("ui_ready addr = %q, want the actual 127.0.0.1 port", sc.addr)
	}
	if code := sc.get(t, "/api/v1/status", nil); code != http.StatusUnauthorized {
		t.Errorf("no token: %d, want 401", code)
	}
	if code := sc.get(t, "/api/v1/status", map[string]string{"Authorization": "Bearer wrong"}); code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d, want 401", code)
	}
	if code := sc.get(t, "/api/v1/status", map[string]string{"Authorization": "Bearer " + token}); code != http.StatusOK {
		t.Errorf("right token: %d, want 200", code)
	}
}

func TestManagementAPI_LoopbackListenerRejectsForeignHostHeader(t *testing.T) {
	sc := startSidecar(t, managementConfig(freePort(t), ""), nil, "-ui-listen", "127.0.0.1:0")

	if code := sc.get(t, "/api/v1/status", map[string]string{"Host": "evil.example:80"}); code != http.StatusForbidden {
		t.Errorf("foreign Host: %d, want 403 (DNS rebinding)", code)
	}
	for _, host := range []string{sc.addr, "localhost:" + strings.Split(sc.addr, ":")[1]} {
		if code := sc.get(t, "/api/v1/status", map[string]string{"Host": host}); code != http.StatusOK {
			t.Errorf("Host %s: %d, want 200", host, code)
		}
	}
}

func TestSidecar_ExitsGracefullyWhenStdinCloses(t *testing.T) {
	modbusPort := freePort(t)
	mmapPath := filepath.Join(t.TempDir(), "sim.bin")
	cfg := strings.Replace(managementConfig(modbusPort, ""), "persistence: { type: memory }",
		fmt.Sprintf("persistence: { type: mmap, path: %q }", mmapPath), 1)

	sc := startSidecar(t, cfg, nil, "-ui-listen", "127.0.0.1:0", "-exit-on-stdin-eof")
	handler, client := newModbusClient(modbusPort, 100)
	if _, err := client.WriteSingleRegister(7, 4321); err != nil {
		t.Fatalf("write: %v", err)
	}
	handler.Close()

	sc.stdin.Close() // what the parent does on quit, or what the OS does if it crashes
	exited := make(chan error, 1)
	go func() { exited <- sc.cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("exit: %v, want status 0", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("still running 5s after stdin closed")
	}
	sawGoodbye := false
	for line := range sc.lines {
		sawGoodbye = sawGoodbye || strings.Contains(line, "Goodbye.")
	}
	if !sawGoodbye {
		t.Error("exited without the normal shutdown path (no Goodbye.)")
	}

	// The value survives in the mmap file across the restart.
	startSidecar(t, cfg, nil, "-ui-listen", "127.0.0.1:0", "-exit-on-stdin-eof")
	handler, client = newModbusClient(modbusPort, 100)
	defer handler.Close()
	got, err := client.ReadHoldingRegisters(7, 1)
	if err != nil || len(got) != 2 || int(got[0])<<8|int(got[1]) != 4321 {
		t.Errorf("after restart: register 7 = %v (err %v), want 4321", got, err)
	}
}

// When the parent dies, nobody reads the sidecar's output any more and the
// OS closes its stdin. Logging the shutdown must not kill it with SIGPIPE
// before persistence is flushed.
func TestSidecar_ShutsDownGracefullyAfterTheParentDied(t *testing.T) {
	modbusPort := freePort(t)
	mmapPath := filepath.Join(t.TempDir(), "sim.bin")
	cfg := strings.Replace(managementConfig(modbusPort, ""), "persistence: { type: memory }",
		fmt.Sprintf("persistence: { type: mmap, path: %q }", mmapPath), 1)
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configFile, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(gatewayBinaryPath, "-config", configFile, "-ui-listen", "127.0.0.1:0", "-exit-on-stdin-eof")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() && !strings.Contains(scanner.Text(), "ui_ready") {
	}
	handler, client := newModbusClient(modbusPort, 100)
	if _, err := client.WriteSingleRegister(7, 4321); err != nil {
		t.Fatalf("write: %v", err)
	}
	handler.Close()

	stdout.Close() // the parent is gone: its read ends close,
	stderr.Close()
	stdin.Close() // and so does the sidecar's stdin
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("exit: %v, want status 0 (a signal means the shutdown was cut short)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("still running 5s after stdin closed")
	}

	startSidecar(t, cfg, nil, "-ui-listen", "127.0.0.1:0", "-exit-on-stdin-eof")
	handler, client = newModbusClient(modbusPort, 100)
	defer handler.Close()
	got, err := client.ReadHoldingRegisters(7, 1)
	if err != nil || len(got) != 2 || int(got[0])<<8|int(got[1]) != 4321 {
		t.Errorf("after restart: register 7 = %v (err %v), want 4321", got, err)
	}
}

func TestSidecar_OpenEventStreamDoesNotDelayShutdown(t *testing.T) {
	sc := startSidecar(t, managementConfig(freePort(t), ""), nil, "-ui-listen", "127.0.0.1:0", "-exit-on-stdin-eof")
	resp, err := http.Get("http://" + sc.addr + "/api/v1/events") // a client left connected
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	start := time.Now()
	sc.stdin.Close()
	exited := make(chan error, 1)
	go func() { exited <- sc.cmd.Wait() }()
	select {
	case <-exited:
		if d := time.Since(start); d > time.Second {
			t.Errorf("shutdown took %v with an event stream open, want < 1s", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("still running 5s after stdin closed")
	}
}
