package test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goburrow/modbus"
	"github.com/goburrow/serial"
	"github.com/tbrandon/mbserver"
)

const (
	gatewayTCPPort = 33502
	pts0           = "/tmp/pts0"
	pts1           = "/tmp/pts1"
	slaveID        = 1
)

var (
	gatewayBinaryPath string
	socatRunnerPath   string
)

// TestMain 是 Go 测试的入口，用于设置和拆除整个测试环境。
func TestMain(m *testing.M) {
	// --- 1. 定位所需文件 ---
	// 假设 go test 在项目根目录执行
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("无法获取当前工作目录: %v", err)
	}
	gatewayBinaryPath = filepath.Join(cwd, "..", "modbus-gateway")
	socatRunnerPath = filepath.Join(cwd, "socat_runner.sh")

	if _, err := os.Stat(gatewayBinaryPath); os.IsNotExist(err) {
		log.Fatalf("modbus-gateway 二进制文件未找到: %s。请先编译项目。", gatewayBinaryPath)
	}
	if _, err := os.Stat(socatRunnerPath); os.IsNotExist(err) {
		log.Fatalf("socat_runner.sh 脚本未找到: %s。", socatRunnerPath)
	}

	// --- 2. 设置测试环境 (Setup) ---
	log.Println("正在启动测试环境...")

	// 启动 socat 创建虚拟串口
	go func() {
		if err := runCommand("socat_runner", socatRunnerPath, "start"); err != nil {
			log.Fatalf("启动 socat 失败: %v", err)
		}
	}()
	// 给予 socat 足够的时间来创建设备文件
	time.Sleep(1 * time.Second)
	log.Println("虚拟串口已创建。")

	// 启动 Modbus RTU 从站模拟器 (mbserver)
	rtuServer := mbserver.NewServer()
	// 预填充一些测试数据
	rtuServer.HoldingRegisters[0] = 12345
	rtuServer.HoldingRegisters[1] = 54321
	rtuServer.Coils[0] = 1 // On
	rtuServer.Coils[1] = 0 // Off
	err = rtuServer.ListenRTU(&serial.Config{Address: pts1, BaudRate: 19200, DataBits: 8, Parity: "N", StopBits: 1})
	if err != nil {
		log.Fatalf("启动 Modbus RTU 从站模拟器失败: %v", err)
	}
	defer rtuServer.Close()
	log.Printf("Modbus RTU 从站已在 %s 上启动。", pts1)

	// Create temporary config file
	configContent := fmt.Sprintf(`
gateways:
  - name: "test-gateway"
    upstreams:
      - type: "tcp"
        tcp:
          address: "0.0.0.0:%d"
    downstreams:
      - name: "rtu-slave"
        type: "rtu"
        slave_ids: "%d"
        serial:
          device: "%s"
          baud_rate: 19200
          data_bits: 8
          parity: "N"
          stop_bits: 1
          timeout: "1s"
log:
  level: "debug"
`, gatewayTCPPort, slaveID, pts0)

	configFile := filepath.Join(cwd, "test_config.yaml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		log.Fatalf("failed to write config file: %v", err)
	}
	defer os.Remove(configFile)

	var gatewayCmd *exec.Cmd
	go func() {
		// 启动 modbus-gateway
		gatewayCmd = exec.Command(gatewayBinaryPath,
			"-config", configFile,
		)
		// 将子进程的标准输出和标准错误重定向到当前测试进程的输出
		gatewayCmd.Stdout = os.Stdout
		gatewayCmd.Stderr = os.Stderr

		if err := gatewayCmd.Start(); err != nil {
			log.Fatalf("启动 modbus-gateway 失败: %v", err)
		}
		log.Printf("modbus-gateway 进程已启动 (PID: %d)，使用配置文件 %s", gatewayCmd.Process.Pid, configFile)
	}()

	// 等待网关完全启动
	time.Sleep(2 * time.Second)

	// --- 3. 运行所有测试 ---
	log.Println("开始执行测试用例...")
	exitCode := m.Run()

	// --- 4. 拆除测试环境 (Teardown) ---
	log.Println("正在清理测试环境...")

	// 停止 modbus-gateway 进程
	if err := gatewayCmd.Process.Kill(); err != nil {
		log.Printf("停止 modbus-gateway 失败: %v", err)
	} else {
		log.Println("modbus-gateway 进程已停止。")
	}

	// 停止 socat
	if err := runCommand("socat_runner", socatRunnerPath, "stop"); err != nil {
		log.Printf("停止 socat 失败: %v", err)
	} else {
		log.Println("socat 进程已停止。")
	}

	os.Exit(exitCode)
}

// runCommand 是一个执行外部命令的辅助函数
func runCommand(name string, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("命令 '%s' 执行失败: %v, 输出: %s", name, err, string(output))
	}
	log.Printf("命令 '%s' 输出:\n%s", name, string(output))
	return nil
}

// newTCPClient 创建并连接一个新的 Modbus TCP 客户端
func newTCPClient(t *testing.T) modbus.Client {
	handler := modbus.NewTCPClientHandler(fmt.Sprintf("127.0.0.1:%d", gatewayTCPPort))
	handler.Timeout = 5 * time.Second
	handler.SlaveId = slaveID

	client := modbus.NewClient(handler)
	err := handler.Connect()
	if err != nil {
		t.Fatalf("无法连接到 modbus-gateway: %v", err)
	}

	// 使用 t.Cleanup 确保连接在每个测试函数结束时关闭
	t.Cleanup(func() {
		handler.Close()
	})

	return client
}

// TestReadHoldingRegisters 测试读取保持寄存器 (FC=03)
func TestReadHoldingRegisters(t *testing.T) {
	client := newTCPClient(t)

	results, err := client.ReadHoldingRegisters(0, 2)
	log.Printf("读取保持寄存器结果: %v", results)
	if err != nil {
		t.Fatalf("读取保持寄存器失败: %v", err)
	}

	if len(results) != 4 { // 2个寄存器 = 4个字节
		t.Fatalf("期望得到4个字节，实际得到 %d", len(results))
	}

	val1 := uint16(results[0])<<8 | uint16(results[1])
	val2 := uint16(results[2])<<8 | uint16(results[3])

	if val1 != 12345 {
		t.Errorf("寄存器 0 的值不匹配。得到: %d, 期望: %d", val1, 12345)
	}
	if val2 != 54321 {
		t.Errorf("寄存器 1 的值不匹配。得到: %d, 期望: %d", val2, 54321)
	}
}

// TestWriteAndReadSingleRegister 测试写入并回读单个寄存器 (FC=06, FC=03)
func TestWriteAndReadSingleRegister(t *testing.T) {
	client := newTCPClient(t)
	const addr = 10
	const valueToWrite uint16 = 0xABCD

	// 写入 (FC=06)
	_, err := client.WriteSingleRegister(addr, valueToWrite)
	if err != nil {
		t.Fatalf("写入单个寄存器失败: %v", err)
	}

	// 等待一小段时间确保数据已通过串口传输并被从站处理
	time.Sleep(100 * time.Millisecond)

	// 回读 (FC=03)
	results, err := client.ReadHoldingRegisters(addr, 1)
	if err != nil {
		t.Fatalf("回读单个寄存器失败: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("期望得到2个字节，实际得到 %d", len(results))
	}

	readValue := uint16(results[0])<<8 | uint16(results[1])
	if readValue != valueToWrite {
		t.Errorf("回读的值不匹配。得到: %#x, 期望: %#x", readValue, valueToWrite)
	}
}

// TestReadTimeoutException 测试下游不响应导致网关返回超时异常 (FC=0x0B)
func TestReadTimeoutException(t *testing.T) {
	// 创建一个特殊的配置文件，指向一个不存在的串口设备以模拟超时/连接失败
	timeoutTCPPort := gatewayTCPPort + 1
	configContent := fmt.Sprintf(`
gateways:
  - name: "timeout-gateway"
    upstreams:
      - type: "tcp"
        tcp:
          address: "127.0.0.1:%d"
    downstreams:
      - name: "timeout-slave"
        type: "rtu"
        slave_ids: "1"
        serial:
          device: "/dev/nonexistent_device_test"
          timeout: "100ms"
`, timeoutTCPPort)

	tmpConfigFile := filepath.Join(os.TempDir(), "timeout_config.yaml")
	os.WriteFile(tmpConfigFile, []byte(configContent), 0644)
	defer os.Remove(tmpConfigFile)

	cmd := exec.Command(gatewayBinaryPath, "-config", tmpConfigFile)
	if err := cmd.Start(); err != nil {
		t.Fatalf("无法启动临时网关: %v", err)
	}
	defer cmd.Process.Kill()

	// 等待网关启动
	time.Sleep(500 * time.Millisecond)

	handler := modbus.NewTCPClientHandler(fmt.Sprintf("127.0.0.1:%d", timeoutTCPPort))
	handler.Timeout = 2 * time.Second
	client := modbus.NewClient(handler)
	if err := handler.Connect(); err != nil {
		// 如果网关因为找不到设备直接退出或拒绝连接，也算一种失败，但我们期望它能运行并返回异常
		t.Logf("连接网关状态: %v", err)
	}
	defer handler.Close()

	_, err := client.ReadHoldingRegisters(0, 1)
	if err == nil {
		t.Fatal("期望得到超时异常响应，但实际成功了")
	}

	log.Printf("捕获到预期的异常: %v", err)
	// 期望错误包含 "exception '11'" (0x0B) 或 "exception '4'" (0x04, 如果是连接失败)
	if !strings.Contains(err.Error(), "exception '11'") && !strings.Contains(err.Error(), "exception '4'") {
		t.Errorf("期望得到 Modbus 异常响应，实际得到: %v", err)
	}
}

