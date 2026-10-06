// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package decode splits Modbus PDUs into labeled fields, each mapped to the
// byte range it occupies, so the UI can link a field to its bytes.
package decode

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// Field is one decoded part of a PDU. Bytes [Start, End) of the PDU hold it;
// Start == End marks a heading with no bytes of its own.
type Field struct {
	Label string
	Value string
	Start int
	End   int
	Depth int // nesting level, for indentation
	Err   bool
}

var functionNames = map[byte]string{
	1: "读线圈", 2: "读离散输入", 3: "读保持", 4: "读输入",
	5: "写单线圈", 6: "写单寄存器", 15: "写多线圈", 16: "写多寄存器",
	22: "掩码写寄存器", 23: "读写多寄存器", 43: "设备标识",
}

// FunctionName names a function code, ignoring the exception bit.
func FunctionName(fc byte) string {
	if name, ok := functionNames[fc&0x7F]; ok {
		return name
	}
	return "未知功能"
}

var exceptionNames = map[byte]string{
	1: "非法功能", 2: "非法数据地址", 3: "非法数据值", 4: "从站设备故障",
	5: "确认", 6: "从站设备忙", 8: "存储奇偶校验错误",
	10: "网关路径不可用", 11: "网关目标设备无响应",
}

type builder struct {
	pdu    []byte
	off    int
	fields []Field
}

func (b *builder) remaining() int { return len(b.pdu) - b.off }

func (b *builder) add(label string, n, depth int, format func([]byte) string) bool {
	if b.remaining() < n {
		b.fields = append(b.fields, Field{Label: label, Value: "长度不足", Start: b.off, End: len(b.pdu), Depth: depth, Err: true})
		b.off = len(b.pdu)
		return false
	}
	v := b.pdu[b.off : b.off+n]
	b.fields = append(b.fields, Field{Label: label, Value: format(v), Start: b.off, End: b.off + n, Depth: depth})
	b.off += n
	return true
}

func (b *builder) u16(label string) (uint16, bool) {
	var v uint16
	ok := b.add(label, 2, 0, func(p []byte) string {
		v = binary.BigEndian.Uint16(p)
		return fmt.Sprintf("%d (0x%04X)", v, v)
	})
	return v, ok
}

func (b *builder) count(label string) (int, bool) {
	var v int
	ok := b.add(label, 1, 0, func(p []byte) string {
		v = int(p[0])
		return fmt.Sprintf("%d", v)
	})
	return v, ok
}

func (b *builder) heading(label, value string) {
	b.fields = append(b.fields, Field{Label: label, Value: value, Start: b.off, End: b.off})
}

// registers decodes n registers numbered from first.
func (b *builder) registers(first uint16, n int) {
	for i := 0; i < n; i++ {
		if !b.add(fmt.Sprintf("%d", int(first)+i), 2, 1, func(p []byte) string {
			v := binary.BigEndian.Uint16(p)
			return fmt.Sprintf("%d (0x%04X)", v, v)
		}) {
			return
		}
	}
}

// bits decodes byteCount bytes of packed bits numbered from first; only the
// first quantity bits are meaningful when quantity > 0.
func (b *builder) bits(first uint16, byteCount, quantity int) {
	for i := 0; i < byteCount; i++ {
		lo := int(first) + i*8
		if !b.add(fmt.Sprintf("%d–%d", lo, lo+7), 1, 1, func(p []byte) string {
			var sb strings.Builder
			for bit := 0; bit < 8; bit++ {
				if quantity > 0 && i*8+bit >= quantity {
					sb.WriteByte('-')
				} else if p[0]&(1<<bit) != 0 {
					sb.WriteByte('1')
				} else {
					sb.WriteByte('0')
				}
			}
			return sb.String() + "（低位在前）"
		}) {
			return
		}
	}
}

func (b *builder) rest(label string) {
	if b.remaining() > 0 {
		b.add(label, b.remaining(), 0, func(p []byte) string { return fmt.Sprintf("% X", p) })
	}
}

func (b *builder) function() (byte, bool) {
	var fc byte
	ok := b.add("功能码", 1, 0, func(p []byte) string {
		fc = p[0]
		if fc&0x80 != 0 {
			return fmt.Sprintf("0x%02X 异常响应（%s）", fc, FunctionName(fc))
		}
		return fmt.Sprintf("0x%02X %s", fc, FunctionName(fc))
	})
	return fc, ok
}

func coilValue(p []byte) string {
	switch binary.BigEndian.Uint16(p) {
	case 0xFF00:
		return "ON (0xFF00)"
	case 0x0000:
		return "OFF (0x0000)"
	}
	return fmt.Sprintf("非法值 0x%04X", binary.BigEndian.Uint16(p))
}

func u16Value(p []byte) string {
	v := binary.BigEndian.Uint16(p)
	return fmt.Sprintf("%d (0x%04X)", v, v)
}

// Request decodes a request PDU.
func Request(pdu []byte) []Field {
	b := &builder{pdu: pdu}
	fc, ok := b.function()
	if !ok {
		return b.fields
	}
	switch fc {
	case 1, 2, 3, 4:
		b.u16("起始地址")
		b.u16("数量")
	case 5:
		b.u16("地址")
		b.add("值", 2, 0, coilValue)
	case 6:
		b.u16("地址")
		b.add("值", 2, 0, u16Value)
	case 15:
		start, _ := b.u16("起始地址")
		qty, _ := b.u16("数量")
		if n, ok := b.count("字节数"); ok {
			b.heading("写入值", "")
			b.bits(start, n, int(qty))
		}
	case 16:
		start, _ := b.u16("起始地址")
		b.u16("数量")
		if n, ok := b.count("字节数"); ok {
			b.heading("写入值", "")
			b.registers(start, n/2)
		}
	case 22:
		b.u16("地址")
		b.add("AND 掩码", 2, 0, u16Value)
		b.add("OR 掩码", 2, 0, u16Value)
	case 23:
		b.u16("读起始地址")
		b.u16("读数量")
		start, _ := b.u16("写起始地址")
		b.u16("写数量")
		if n, ok := b.count("字节数"); ok {
			b.heading("写入值", "")
			b.registers(start, n/2)
		}
	}
	b.rest("数据")
	return b.fields
}

// Response decodes a response PDU; req, the matching request PDU, supplies
// the addresses that a response leaves implicit. It may be nil.
func Response(pdu, req []byte) []Field {
	b := &builder{pdu: pdu}
	fc, ok := b.function()
	if !ok {
		return b.fields
	}
	if fc&0x80 != 0 {
		b.add("异常码", 1, 0, func(p []byte) string {
			name, ok := exceptionNames[p[0]]
			if !ok {
				name = "未知异常"
			}
			return fmt.Sprintf("0x%02X %s", p[0], name)
		})
		if n := len(b.fields); n > 0 {
			b.fields[n-1].Err = true
		}
		b.rest("数据")
		return b.fields
	}
	start, quantity := requestRange(req, fc)
	switch fc {
	case 1, 2:
		if n, ok := b.count("字节数"); ok {
			b.heading("状态", "")
			b.bits(start, n, int(quantity))
		}
	case 3, 4, 23:
		if n, ok := b.count("字节数"); ok {
			b.heading("寄存器", "")
			b.registers(start, n/2)
		}
	case 5:
		b.u16("地址")
		b.add("值", 2, 0, coilValue)
	case 6:
		b.u16("地址")
		b.add("值", 2, 0, u16Value)
	case 15, 16:
		b.u16("起始地址")
		b.u16("数量")
	case 22:
		b.u16("地址")
		b.add("AND 掩码", 2, 0, u16Value)
		b.add("OR 掩码", 2, 0, u16Value)
	}
	b.rest("数据")
	return b.fields
}

// requestRange returns the start address and quantity a response's data
// refers to, or zeros when req does not carry them. For function 23 that is
// the read range, which leads the request just as for a plain read.
func requestRange(req []byte, fc byte) (start, quantity uint16) {
	if len(req) < 5 || req[0] != fc {
		return 0, 0
	}
	return binary.BigEndian.Uint16(req[1:3]), binary.BigEndian.Uint16(req[3:5])
}
