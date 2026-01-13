package test

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goburrow/modbus"
)

const (
	perfGatewayPort = 33503
	perfSlaveID     = 1
)

func TestPerformance_ModbusTCP_LocalSlave(t *testing.T) {
	// 1. Prepare Environment
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get CWD: %v", err)
	}
	gatewayBin := filepath.Join(cwd, "..", "modbus-gateway")
	if _, err := os.Stat(gatewayBin); os.IsNotExist(err) {
		t.Fatalf("Gateway binary not found at %s. Build it first.", gatewayBin)
	}

	// 2. Generate Config
	// Using memory persistence for high-throughput testing
	configContent := fmt.Sprintf(`
gateways:
  - name: "perf-gw"
    upstreams:
      - type: "tcp"
        tcp:
          address: "0.0.0.0:%d"
    downstreams:
      - name: "local-slave"
        type: "local"
        slave_ids: "%d"
        local:
          persistence:
            type: "memory"
            path: "/tmp/a.bin"
log:
  level: "info"
`, perfGatewayPort, perfSlaveID)

	configFile := filepath.Join(cwd, "perf_config.yaml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	defer os.Remove(configFile)

	// 3. Start Gateway
	cmd := exec.Command(gatewayBin, "-config", configFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start gateway: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Allow startup time
	time.Sleep(2 * time.Second)

	// 4. Test Variables
	var (
		writeOps     int64
		writeErrs    int64
		readOps      int64
		readErrs     int64
		testDuration = 10 * time.Second // Run for 10 seconds
	)

	// Client Factory
	newClient := func() modbus.Client {
		handler := modbus.NewTCPClientHandler(fmt.Sprintf("127.0.0.1:%d", perfGatewayPort))
		handler.SlaveId = perfSlaveID
		handler.Timeout = 1 * time.Second // Tight timeout for performance testing
		handler.Connect()
		return modbus.NewClient(handler)
	}

	wg := sync.WaitGroup{}
	start := time.Now()

	// 5. Write Routine: Every 3 seconds, 300 writes
	wg.Add(1)
	go func() {
		defer wg.Done()
		client := newClient()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		// Initial burst immediately? Or wait for first tick?
		// "Every 3 seconds" usually implies T=0, T=3, etc. or T=3, T=6.
		// Let's do T=0 first manually if needed, but ticker fires after duration.
		// Let's rely on ticker.

		timer := time.NewTimer(testDuration)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				return
			case <-ticker.C:
				// Burst 300 writes
				for i := 0; i < 300; i++ {
					// Random address 0-999
					addr := uint16(rand.Intn(1000))
					val := uint16(rand.Intn(65535))
					wStart := time.Now().UnixMicro()
					_, err := client.WriteSingleRegister(addr, val)
					t.Logf("WriteSingleRegister use %v μs", time.Now().UnixMicro()-wStart)
					if err != nil {
						atomic.AddInt64(&writeErrs, 1)
						// Log only first few errors to avoid spam
						if atomic.LoadInt64(&writeErrs) <= 5 {
							t.Logf("Write Error: %v", err)
						}
					} else {
						atomic.AddInt64(&writeOps, 1)
					}
				}
			}
		}
	}()

	// 6. Read Routine: Every 2 seconds, continuous read
	// "Continuous read" could mean "start reading constantly for a while" or "read once".
	// Given "Every 2 seconds happen continuous read", I'll assume: trigger a read of a block.
	// Or maybe it means "read continuously"? No, "Every 2s happen..." implies an event.
	// I will read a large block (100 registers) to simulate a polling cycle.
	wg.Add(1)
	go func() {
		defer wg.Done()
		client := newClient()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		timer := time.NewTimer(testDuration)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				return
			case <-ticker.C:
				// Read 100 registers starting at 0
				rStart := time.Now().UnixMicro()
				_, err := client.ReadHoldingRegisters(0, 100)
				t.Logf("ReadHoldingRegisters use %v μs", time.Now().UnixMicro()-rStart)
				if err != nil {
					atomic.AddInt64(&readErrs, 1)
					if atomic.LoadInt64(&readErrs) <= 5 {
						t.Logf("Read Error: %v", err)
					}
				} else {
					atomic.AddInt64(&readOps, 1)
				}
			}
		}
	}()

	// Wait for test duration
	wg.Wait()
	duration := time.Since(start)

	// 7. Report
	t.Logf("Test Finished in %v", duration)
	t.Logf("Total Writes: %d (Errors: %d)", atomic.LoadInt64(&writeOps), atomic.LoadInt64(&writeErrs))
	t.Logf("Total Reads: %d (Errors: %d)", atomic.LoadInt64(&readOps), atomic.LoadInt64(&readErrs))

	if atomic.LoadInt64(&writeErrs) > 0 || atomic.LoadInt64(&readErrs) > 0 {
		t.Errorf("Performance test failed with errors.")
	}
}
