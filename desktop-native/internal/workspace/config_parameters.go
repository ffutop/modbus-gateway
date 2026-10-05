package workspace

import (
	"fmt"
	"github.com/ffutop/modbus-gateway/internal/config"
	"strconv"
	"time"
)

func boolSpec(path, label string, p *bool) *spec {
	return &spec{path: path, label: label, options: []string{"开启", "关闭"}, get: func() string {
		if *p {
			return "开启"
		}
		return "关闭"
	}, set: func(v string) { *p = v == "开启" }}
}
func durationSpec(path, label string, p *time.Duration) *spec {
	return &spec{path: path, label: label, get: func() string { return p.String() }, set: func(v string) {
		if n, err := time.ParseDuration(v); err == nil {
			*p = n
		}
	}, check: func(v string) string {
		n, err := time.ParseDuration(v)
		if err != nil || n < 0 {
			return "请输入非负时长，例如 500ms、2s"
		}
		return ""
	}}
}
func serialSpecs(base string, s *config.SerialConfig, when func() bool) []*spec {
	parity := strSpec(base+".serial.parity", "校验位", &s.Parity)
	parity.options = []string{"N", "E", "O"}
	specs := []*spec{intSpec(base+".serial.data_bits", "数据位", &s.DataBits, []string{"5", "6", "7", "8"}), parity, intSpec(base+".serial.stop_bits", "停止位", &s.StopBits, []string{"1", "2"}), durationSpec(base+".serial.timeout", "响应超时", &s.Timeout), durationSpec(base+".serial.rqst_pause", "请求间隔", &s.RqstPause), boolSpec(base+".serial.rs485", "RS485", &s.RS485), durationSpec(base+".serial.delay_rts_before_send", "发送前 RTS 延迟", &s.DelayRtsBeforeSend), durationSpec(base+".serial.delay_rts_after_send", "发送后 RTS 延迟", &s.DelayRtsAfterSend), boolSpec(base+".serial.rts_high_during_send", "发送时 RTS 高电平", &s.RtsHighDuringSend), boolSpec(base+".serial.rts_high_after_send", "发送后 RTS 高电平", &s.RtsHighAfterSend), boolSpec(base+".serial.rx_during_tx", "发送时接收", &s.RxDuringTx)}
	specs[4].hint = "当前 RTU 驱动尚未应用请求间隔；此值仅保留在 YAML 中"
	specs[5].hint = "当前 RTU 驱动尚未接入 RS485 / RTS 参数；方向控制仍依赖设备或适配器"
	for i, sp := range specs {
		sp.when = when
		if i >= 6 {
			sp.when = func() bool { return when() && s.RS485 }
		}
	}
	return specs
}
func globalAdvanced(c *config.Config) []*spec {
	on := boolSpec("pprof.enabled", "性能诊断", &c.Pprof.Enabled)
	addr := strSpec("pprof.address", "诊断监听地址", &c.Pprof.Address)
	addr.when = func() bool { return c.Pprof.Enabled }
	addr.check = checkAddr
	on.advanced = true
	addr.advanced = true
	return []*spec{on, addr}
}
func writeSerial(p func(int, string, ...any), indent int, s config.SerialConfig) {
	p(indent, "data_bits: %d", s.DataBits)
	p(indent, "parity: %s", q(s.Parity))
	p(indent, "stop_bits: %d", s.StopBits)
	p(indent, "timeout: %s", s.Timeout)
	p(indent, "rqst_pause: %s", s.RqstPause)
	p(indent, "rs485: %t", s.RS485)
	p(indent, "delay_rts_before_send: %s", s.DelayRtsBeforeSend)
	p(indent, "delay_rts_after_send: %s", s.DelayRtsAfterSend)
	p(indent, "rts_high_during_send: %t", s.RtsHighDuringSend)
	p(indent, "rts_high_after_send: %t", s.RtsHighAfterSend)
	p(indent, "rx_during_tx: %t", s.RxDuringTx)
}
func validatedInt(path, label string, p *int) *spec {
	s := intSpec(path, label, p, nil)
	s.check = func(v string) string {
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Sprintf("%s必须是整数", label)
		}
		return ""
	}
	return s
}

func baudSpec(path string, p *int) *spec {
	s := validatedInt(path, "波特率", p)
	s.hint = "常用 9600、19200、38400、115200；也可输入设备支持的其他值"
	s.check = func(v string) string {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return "波特率必须是正整数"
		}
		return ""
	}
	return s
}
