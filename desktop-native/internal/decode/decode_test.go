// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package decode

import (
	"testing"
)

type want struct {
	label, value string
	start, end   int
}

func check(t *testing.T, got []Field, wants []want) {
	t.Helper()
	if len(got) != len(wants) {
		t.Fatalf("got %d fields %+v, want %d", len(got), got, len(wants))
	}
	for i, w := range wants {
		g := got[i]
		if g.Label != w.label || g.Value != w.value || g.Start != w.start || g.End != w.end {
			t.Errorf("field %d = {%q %q [%d,%d)}, want {%q %q [%d,%d)}", i, g.Label, g.Value, g.Start, g.End, w.label, w.value, w.start, w.end)
		}
	}
}

func TestReadHoldingRegisters(t *testing.T) {
	req := []byte{0x03, 0x00, 0x10, 0x00, 0x02}
	check(t, Request(req), []want{
		{"功能码", "0x03 读保持", 0, 1},
		{"起始地址", "16 (0x0010)", 1, 3},
		{"数量", "2 (0x0002)", 3, 5},
	})
	check(t, Response([]byte{0x03, 0x04, 0x00, 0x0A, 0x12, 0x34}, req), []want{
		{"功能码", "0x03 读保持", 0, 1},
		{"字节数", "4", 1, 2},
		{"寄存器", "", 2, 2},
		{"16", "10 (0x000A)", 2, 4},
		{"17", "4660 (0x1234)", 4, 6},
	})
}

func TestReadCoilsMarksBitsBeyondQuantity(t *testing.T) {
	req := []byte{0x01, 0x00, 0x08, 0x00, 0x03}
	check(t, Response([]byte{0x01, 0x01, 0x05}, req), []want{
		{"功能码", "0x01 读线圈", 0, 1},
		{"字节数", "1", 1, 2},
		{"状态", "", 2, 2},
		{"8–15", "101-----（低位在前）", 2, 3},
	})
}

func TestWriteMultipleRegisters(t *testing.T) {
	check(t, Request([]byte{0x10, 0x00, 0x01, 0x00, 0x01, 0x02, 0xAB, 0xCD}), []want{
		{"功能码", "0x10 写多寄存器", 0, 1},
		{"起始地址", "1 (0x0001)", 1, 3},
		{"数量", "1 (0x0001)", 3, 5},
		{"字节数", "2", 5, 6},
		{"写入值", "", 6, 6},
		{"1", "43981 (0xABCD)", 6, 8},
	})
}

func TestWriteSingleCoil(t *testing.T) {
	check(t, Request([]byte{0x05, 0x00, 0x07, 0xFF, 0x00}), []want{
		{"功能码", "0x05 写单线圈", 0, 1},
		{"地址", "7 (0x0007)", 1, 3},
		{"值", "ON (0xFF00)", 3, 5},
	})
}

func TestExceptionResponse(t *testing.T) {
	got := Response([]byte{0x83, 0x02}, nil)
	check(t, got, []want{
		{"功能码", "0x83 异常响应（读保持）", 0, 1},
		{"异常码", "0x02 非法数据地址", 1, 2},
	})
	if !got[1].Err {
		t.Error("exception code should be flagged as an error")
	}
}

func TestTruncatedPDU(t *testing.T) {
	got := Request([]byte{0x03, 0x00})
	check(t, got, []want{
		{"功能码", "0x03 读保持", 0, 1},
		{"起始地址", "长度不足", 1, 2},
		{"数量", "长度不足", 2, 2},
	})
	if !got[1].Err {
		t.Error("truncated field should be flagged as an error")
	}
}

func TestUnknownFunctionKeepsRawData(t *testing.T) {
	check(t, Request([]byte{0x41, 0x01, 0x02}), []want{
		{"功能码", "0x41 未知功能", 0, 1},
		{"数据", "01 02", 1, 3},
	})
}

func TestEmptyPDU(t *testing.T) {
	check(t, Request(nil), []want{{"功能码", "长度不足", 0, 0}})
}
