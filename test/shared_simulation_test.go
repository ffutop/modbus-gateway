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

// startGatewayWithConfig writes configContent to a temp file, starts the
// gateway binary against it, and returns a cleanup func that stops it.
func startGatewayWithConfig(t *testing.T, name, configContent string) func() {
	t.Helper()

	configFile := filepath.Join(os.TempDir(), name+".yaml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cmd := exec.Command(gatewayBinaryPath, "-config", configFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start gateway: %v", err)
	}
	time.Sleep(1 * time.Second)

	return func() {
		cmd.Process.Signal(os.Interrupt)
		cmd.Wait()
		os.Remove(configFile)
	}
}

func sharedSimulationConfig(businessPort, injectionPort int, persistenceType, persistencePath string) string {
	persistenceBlock := fmt.Sprintf("type: %s", persistenceType)
	if persistencePath != "" {
		persistenceBlock += fmt.Sprintf("\n      path: %q", persistencePath)
	}
	return fmt.Sprintf(`
version: 1

simulations:
  - name: sim-device
    persistence:
      %s

gateways:
  - name: business-modbus
    upstreams:
      - type: tcp
        tcp: { address: "0.0.0.0:%d" }
    downstreams:
      - type: local
        slave_ids: "100"
        simulation: { ref: sim-device }

  - name: simulation-injection
    upstreams:
      - type: tcp
        tcp: { address: "0.0.0.0:%d" }
    downstreams:
      - type: injector
        slave_ids: "247"
        simulation:
          ref: sim-device
          mappings:
            - source: { table: coils, start_address: 0, count: 16 }
              target: { table: discrete_inputs, start_address: 0 }
            - source: { table: holding_registers, start_address: 0, count: 8 }
              target: { table: input_registers, start_address: 0 }
log:
  level: "debug"
`, persistenceBlock, businessPort, injectionPort)
}

func newModbusClient(port int, slaveID byte) (*modbus.TCPClientHandler, modbus.Client) {
	handler := modbus.NewTCPClientHandler(fmt.Sprintf("127.0.0.1:%d", port))
	handler.Timeout = 1 * time.Second
	handler.SlaveId = slaveID
	return handler, modbus.NewClient(handler)
}

func TestSharedSimulation_InjectionFillsDiscreteAndInputRegisters(t *testing.T) {
	businessPort, injectionPort := 34501, 34502
	defer startGatewayWithConfig(t, "shared_sim_basic", sharedSimulationConfig(businessPort, injectionPort, "memory", ""))()

	businessHandler, businessClient := newModbusClient(businessPort, 100)
	if err := businessHandler.Connect(); err != nil {
		t.Fatalf("business connect failed: %v", err)
	}
	defer businessHandler.Close()

	injectionHandler, injectionClient := newModbusClient(injectionPort, 247)
	if err := injectionHandler.Connect(); err != nil {
		t.Fatalf("injection connect failed: %v", err)
	}
	defer injectionHandler.Close()

	// Business writes its own Coils/Holding Registers directly (unaffected by injection).
	if _, err := businessClient.WriteSingleCoil(0, 0xFF00); err != nil {
		t.Fatalf("business WriteSingleCoil failed: %v", err)
	}

	// Injector fills Discrete Inputs via FC15 on Coils source range [0,16).
	if _, err := injectionClient.WriteMultipleCoils(0, 16, []byte{0xFF, 0x00}); err != nil {
		t.Fatalf("injector WriteMultipleCoils failed: %v", err)
	}
	// Injector fills Input Registers via FC16 on HoldingRegisters source range [0,8).
	if _, err := injectionClient.WriteMultipleRegisters(0, 2, []byte{0x00, 0x2A, 0x01, 0x00}); err != nil {
		t.Fatalf("injector WriteMultipleRegisters failed: %v", err)
	}

	// Business reads the coil it wrote itself.
	coils, err := businessClient.ReadCoils(0, 1)
	if err != nil || coils[0] != 1 {
		t.Errorf("expected business coil 0 to be 1, got %v err=%v", coils, err)
	}

	// Business reads Discrete Inputs filled by the injector (FC02).
	discretes, err := businessClient.ReadDiscreteInputs(0, 16)
	if err != nil {
		t.Fatalf("business ReadDiscreteInputs failed: %v", err)
	}
	if discretes[0] != 0xFF || discretes[1] != 0x00 {
		t.Errorf("expected discrete inputs 0xFF,0x00, got %x", discretes)
	}

	// Business reads Input Registers filled by the injector (FC04).
	inputs, err := businessClient.ReadInputRegisters(0, 2)
	if err != nil {
		t.Fatalf("business ReadInputRegisters failed: %v", err)
	}
	if inputs[0] != 0x00 || inputs[1] != 0x2A || inputs[2] != 0x01 || inputs[3] != 0x00 {
		t.Errorf("expected input registers 0x002A,0x0100, got %x", inputs)
	}
}

func TestSharedSimulation_InjectorRejectsUnmappedAndReadRequests(t *testing.T) {
	businessPort, injectionPort := 34503, 34504
	defer startGatewayWithConfig(t, "shared_sim_reject", sharedSimulationConfig(businessPort, injectionPort, "memory", ""))()

	injectionHandler, injectionClient := newModbusClient(injectionPort, 247)
	if err := injectionHandler.Connect(); err != nil {
		t.Fatalf("injection connect failed: %v", err)
	}
	defer injectionHandler.Close()

	// Address 20 is outside the mapped coil range [0,16).
	if _, err := injectionClient.WriteSingleCoil(20, 0xFF00); err == nil {
		t.Error("expected error writing to an unmapped address, got success")
	} else if !strings.Contains(err.Error(), "exception '2'") {
		t.Logf("got error as expected (illegal data address): %v", err)
	}

	// Reads are never valid on the injector entry.
	if _, err := injectionClient.ReadCoils(0, 1); err == nil {
		t.Error("expected error reading via the injector entry, got success")
	} else if !strings.Contains(err.Error(), "exception '1'") {
		t.Logf("got error as expected (illegal function): %v", err)
	}
}

func TestSharedSimulation_RouteIsolationBetweenBusinessAndInjection(t *testing.T) {
	businessPort, injectionPort := 34505, 34506
	defer startGatewayWithConfig(t, "shared_sim_isolation", sharedSimulationConfig(businessPort, injectionPort, "memory", ""))()

	// Slave 247 (injection ID) is unreachable through the business port.
	businessAsInjector, _ := newModbusClient(businessPort, 247)
	if err := businessAsInjector.Connect(); err == nil {
		defer businessAsInjector.Close()
	}
	businessInjectorClient := modbus.NewClient(businessAsInjector)
	if _, err := businessInjectorClient.ReadCoils(0, 1); err == nil {
		t.Error("expected slave 247 to be unreachable via the business port")
	}

	// Slave 100 (business ID) is unreachable through the injection port.
	injectionAsBusiness, _ := newModbusClient(injectionPort, 100)
	if err := injectionAsBusiness.Connect(); err == nil {
		defer injectionAsBusiness.Close()
	}
	injectionBusinessClient := modbus.NewClient(injectionAsBusiness)
	if _, err := injectionBusinessClient.ReadCoils(0, 1); err == nil {
		t.Error("expected slave 100 to be unreachable via the injection port")
	}
}

func TestSharedSimulation_PersistenceRestartRecoversInjectedData(t *testing.T) {
	businessPort, injectionPort := 34507, 34508
	dbPath := filepath.Join(os.TempDir(), "shared_sim_persist.bin")
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	configContent := sharedSimulationConfig(businessPort, injectionPort, "mmap", dbPath)

	stop := startGatewayWithConfig(t, "shared_sim_persist_run1", configContent)

	injectionHandler, injectionClient := newModbusClient(injectionPort, 247)
	if err := injectionHandler.Connect(); err != nil {
		t.Fatalf("injection connect failed: %v", err)
	}
	if _, err := injectionClient.WriteMultipleRegisters(0, 1, []byte{0xBE, 0xEF}); err != nil {
		t.Fatalf("injector WriteMultipleRegisters failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	injectionHandler.Close()
	stop()

	// Restart and verify the business side reads back the injected value.
	defer startGatewayWithConfig(t, "shared_sim_persist_run2", configContent)()

	businessHandler, businessClient := newModbusClient(businessPort, 100)
	if err := businessHandler.Connect(); err != nil {
		t.Fatalf("business connect failed: %v", err)
	}
	defer businessHandler.Close()

	inputs, err := businessClient.ReadInputRegisters(0, 1)
	if err != nil {
		t.Fatalf("business ReadInputRegisters failed: %v", err)
	}
	if inputs[0] != 0xBE || inputs[1] != 0xEF {
		t.Errorf("expected restored input register 0xBEEF, got %x", inputs)
	}
}
