package test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goburrow/modbus"
)

func TestLocalSlave(t *testing.T) {
	// 1. Config
	localPort := 33503
	configContent := fmt.Sprintf(`
gateways:
  - name: "local-gateway"
    upstreams:
      - type: "tcp"
        tcp:
          address: "0.0.0.0:%d"
    downstreams:
      - name: "local-device"
        type: "local"
        slave_ids: "1"
log:
  level: "debug"
`, localPort)

	tmpConfigFile := filepath.Join(os.TempDir(), "local_slave_config.yaml")
	if err := os.WriteFile(tmpConfigFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	defer os.Remove(tmpConfigFile)

	// 2. Start Gateway
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get CWD: %v", err)
	}
	gatewayBinaryPath := filepath.Join(cwd, "..", "modbus-gateway")
	if _, err := os.Stat(gatewayBinaryPath); os.IsNotExist(err) {
		t.Fatalf("Gateway binary not found at %s. Build it first.", gatewayBinaryPath)
	}

	cmd := exec.Command(gatewayBinaryPath, "-config", tmpConfigFile)
	// cmd.Stdout = os.Stdout // Uncomment for debug
	// cmd.Stderr = os.Stderr // Uncomment for debug

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start gateway: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for start
	time.Sleep(1 * time.Second)

	// 3. Client
	handler := modbus.NewTCPClientHandler(fmt.Sprintf("127.0.0.1:%d", localPort))
	handler.Timeout = 1 * time.Second
	handler.SlaveId = 1
	client := modbus.NewClient(handler)
	if err := handler.Connect(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer handler.Close()

	// 4. Test Coils
	t.Log("Testing Write/Read Coil 0")
	// Write Coil 0 -> On
	_, err = client.WriteSingleCoil(0, 0xFF00)
	if err != nil {
		t.Errorf("WriteSingleCoil failed: %v", err)
	}
	// Read Coil 0
	results, err := client.ReadCoils(0, 1)
	if err != nil {
		t.Errorf("ReadCoils failed: %v", err)
	}
	if len(results) > 0 && results[0] != 1 {
		t.Errorf("Expected coil 0 to be 1, got %v", results[0])
	}

	// Write Coil 0 -> Off
	_, err = client.WriteSingleCoil(0, 0x0000)
	if err != nil {
		t.Errorf("WriteSingleCoil failed: %v", err)
	}
	// Read Coil 0
	results, err = client.ReadCoils(0, 1)
	if err != nil {
		t.Errorf("ReadCoils failed: %v", err)
	}
	if len(results) > 0 && results[0] != 0 {
		t.Errorf("Expected coil 0 to be 0, got %v", results[0])
	}

	// 5. Test Registers
	t.Log("Testing Write/Read Register 10")
	// Write Reg 10 -> 12345
	_, err = client.WriteSingleRegister(10, 12345)
	if err != nil {
		t.Errorf("WriteSingleRegister failed: %v", err)
	}
	// Read Reg 10
	results, err = client.ReadHoldingRegisters(10, 1)
	if err != nil {
		t.Errorf("ReadHoldingRegisters failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 bytes, got %d", len(results))
	}
	val := uint16(results[0])<<8 | uint16(results[1])
	if val != 12345 {
		t.Errorf("Expected reg 10 to be 12345, got %d", val)
	}

	// 6. Test Routing Error (Wrong ID)
	t.Log("Testing Routing Error")
	handler2 := modbus.NewTCPClientHandler(fmt.Sprintf("127.0.0.1:%d", localPort))
	handler2.Timeout = 1 * time.Second
	handler2.SlaveId = 2 // Not in "1"
	client2 := modbus.NewClient(handler2)
	handler2.Connect()
	defer handler2.Close()

	_, err = client2.ReadCoils(0, 1)
	if err == nil {
		t.Error("Expected error for wrong Slave ID, got success")
	} else {
		// Expected "gateway path unavailable" or similar
		// Check for exception code 10 (0x0A) or timeout
		if strings.Contains(err.Error(), "exception '10'") {
			t.Logf("Got expected Gateway Path Unavailable exception: %v", err)
		} else {
			t.Logf("Got error as expected (but maybe not explicit 10?): %v", err)
		}
	}
}
