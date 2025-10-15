package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config 结构体定义了网关的所有配置项
type Config struct {
	// TCP Server 配置
	TCPAddress string `mapstructure:"tcp_address"` // TCP 监听地址, e.g., "0.0.0.0"
	TCPPort    int    `mapstructure:"tcp_port"`    // TCP 监听端口, e.g., 502
	MaxConns   int    `mapstructure:"max_conns"`   // 最大并发 TCP 连接数

	// Serial/RTU 配置
	Device    string        `mapstructure:"device"`     // 串口设备, e.g., "/dev/ttyUSB0"
	BaudRate  int           `mapstructure:"baud_rate"`  // 波特率
	DataBits  int           `mapstructure:"data_bits"`  // 数据位
	Parity    string        `mapstructure:"parity"`     // 校验位 (N, E, O)
	StopBits  int           `mapstructure:"stop_bits"`  // 停止位
	Timeout   time.Duration `mapstructure:"timeout"`    // RTU 响应超时
	RqstPause time.Duration `mapstructure:"rqst_pause"` // 两个请求之间的间隔

	// Serial/RTU RS485 配置
	RS485              bool          `mapstructure:"rs485"`                 // RS485 配置
	DelayRtsBeforeSend time.Duration `mapstructure:"delay_rts_before_send"` // 发送前延迟 RTS 时间
	DelayRtsAfterSend  time.Duration `mapstructure:"delay_rts_after_send"`  // 发送后延迟 RTS 时间
	RtsHighDuringSend  bool          `mapstructure:"rts_high_during_send"`  // 设置在发送过程中保持 RTS 高电平
	RtsHighAfterSend   bool          `mapstructure:"rts_high_after_send"`   // 设置在发送后保持 RTS 高电平
	RxDuringTx         bool          `mapstructure:"rx_during_tx"`          // 设置在发送过程中保持 RTS 高电平

	// 网关配置
	LogLevel string `mapstructure:"log_level"` // 日志级别 (debug, info, warn, error)
	LogFile  string `mapstructure:"log_file"`  // 日志文件路径, 为空则输出到标准输出

	// 内部使用的配置
	ConfigFile string `mapstructure:"-"` // 配置文件路径，不从配置文件中读取
}

// LoadConfig 从命令行和配置文件加载配置
func LoadConfig() (*Config, error) {
	// 1. 设置默认值
	viper.SetDefault("tcp_address", "0.0.0.0")
	viper.SetDefault("tcp_port", 502)
	viper.SetDefault("max_conns", 32)
	viper.SetDefault("max_retries", 0)
	viper.SetDefault("device", "/tmp/pts1") // 默认使用虚拟串口，方便测试
	viper.SetDefault("baud_rate", 19200)
	viper.SetDefault("data_bits", 8)
	viper.SetDefault("parity", "N")
	viper.SetDefault("stop_bits", 1)
	viper.SetDefault("timeout", 500*time.Millisecond)
	viper.SetDefault("rqst_pause", 100*time.Millisecond)
	viper.SetDefault("log_level", "info")
	viper.SetDefault("log_file", "") // 默认输出到 stdout

	// 2. 定义命令行参数
	pflag.StringP("config", "c", "", "Configuration file path.")
	pflag.StringP("tcp_address", "A", viper.GetString("tcp_address"), "TCP server address to bind.")
	pflag.IntP("tcp_port", "P", viper.GetInt("tcp_port"), "TCP server port number.")
	pflag.IntP("max_conns", "C", viper.GetInt("max_conns"), "Maximum number of simultaneous TCP connections.")
	pflag.IntP("max_retries", "N", viper.GetInt("max_retries"), "Maximum number of retries.")
	pflag.StringP("device", "p", viper.GetString("device"), "Serial port device name.")
	pflag.IntP("baud_rate", "s", viper.GetInt("baud_rate"), "Serial port speed.")
	pflag.DurationP("timeout", "W", viper.GetDuration("timeout"), "Response wait time.")
	pflag.DurationP("rqst_pause", "R", viper.GetDuration("rqst_pause"), "Pause between requests.")
	pflag.StringP("log_level", "v", viper.GetString("log_level"), "Log verbosity level (debug, info, warn, error).")
	pflag.StringP("log_file", "L", viper.GetString("log_file"), "Log file name ('-' for logging to STDOUT only).")
	pflag.Parse()

	// 3. 将 pflag 绑定到 viper
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		return nil, fmt.Errorf("failed to bind pflags: %w", err)
	}

	// 4. 读取配置文件
	configFile := viper.GetString("config")
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("config")          // 配置文件名 (不带扩展名)
		viper.SetConfigType("yaml")            // 显式设置配置文件类型
		viper.AddConfigPath("/etc/modbusgw/")  // /etc/modbusgw/config.yaml
		viper.AddConfigPath("$HOME/.modbusgw") // $HOME/.modbusgw/config.yaml
		viper.AddConfigPath(".")               // ./config.yaml
	}

	// 查找并读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		// 如果错误是 "配置文件未找到"，可以忽略，因为配置可以通过命令行参数提供
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// 5. 将配置 unmarshal 到结构体
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 修正 Parity 字符串为大写
	config.Parity = strings.ToUpper(config.Parity)

	return &config, nil
}
