package test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/goburrow/modbus"
)

func TestPersistence(t *testing.T) {
	// 1. Setup paths
	tempDir := os.TempDir()
	dbPath := filepath.Join(tempDir, "modbus_test.json")
	os.Remove(dbPath) // Ensure clean start
	defer os.Remove(dbPath)

	port := 33504
	configContent := fmt.Sprintf(`
gateways:
  - name: "persist-gw"
    upstreams:
      - type: "tcp"
        tcp:
          address: "0.0.0.0:%d"
    downstreams:
      - name: "local-db"
        type: "local"
        slave_ids: "1"
        local:
          persistence:
            type: "file"
            path: "%s"
            interval: "100ms"
log:
  level: "debug"
`, port, dbPath)

	configFile := filepath.Join(tempDir, "persist_config.yaml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	defer os.Remove(configFile)

	// 2. Helper to run gateway
	runGateway := func() *exec.Cmd {
		cwd, _ := os.Getwd()
		binPath := filepath.Join(cwd, "..", "modbus-gateway")
		cmd := exec.Command(binPath, "-config", configFile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("Failed to start gateway: %v", err)
		}
		return cmd
	}

	// 3. First Run: Write Data
	t.Log("Starting Gateway (Run 1)...")
	cmd1 := runGateway()
	time.Sleep(1 * time.Second) // Wait for start

	handler := modbus.NewTCPClientHandler(fmt.Sprintf("127.0.0.1:%d", port))
	handler.SlaveId = 1
	client := modbus.NewClient(handler)
	handler.Connect()

	t.Log("Writing 0xCAFE to Register 10...")
	if _, err := client.WriteSingleRegister(10, 0xCAFE); err != nil {
		cmd1.Process.Kill()
		t.Fatalf("Write failed: %v", err)
	}

	// Wait for auto-save (interval is 100ms)
	time.Sleep(500 * time.Millisecond)

	// Stop Gateway 1
	t.Log("Stopping Gateway (Run 1)...")
	handler.Close()
	cmd1.Process.Signal(os.Interrupt)
	cmd1.Wait()

	// Verify file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("Persistence file was not created at %s", dbPath)
	}

	// 4. Second Run: Verify Data
	t.Log("Starting Gateway (Run 2)...")
	cmd2 := runGateway()
	defer func() {
		cmd2.Process.Kill()
		cmd2.Wait()
	}()
	time.Sleep(1 * time.Second)

	handler2 := modbus.NewTCPClientHandler(fmt.Sprintf("127.0.0.1:%d", port))
	handler2.SlaveId = 1
	client2 := modbus.NewClient(handler2)
	handler2.Connect()
	defer handler2.Close()

	t.Log("Reading Register 10...")
	results, err := client2.ReadHoldingRegisters(10, 1)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	val := uint16(results[0])<<8 | uint16(results[1])
	if val != 0xCAFE {
		t.Errorf("Expected 0xCAFE, got 0x%X", val)
	} else {
		t.Log("Persistence verified: 0xCAFE matches.")
	}
}
