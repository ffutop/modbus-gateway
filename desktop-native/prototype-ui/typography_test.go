package main

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
	"testing"
)

func TestSingleLineTypography(t *testing.T) {
	th := NewTheme()
	for _, scale := range []float32{1, 1.5, 2} {
		var reference D
		for i, txt := range []string{"文件", "编辑", "查看", "网关", "ModMux", "ID 10 输入寄存器"} {
			var ops op.Ops
			gtx := C{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(1000, 100)}}
			d := th.label(txt, textSize, colInk).Layout(gtx)
			if i == 0 {
				reference = d
			}
			if d.Size.Y != reference.Size.Y || d.Baseline != reference.Baseline {
				t.Errorf("scale=%g %q height/baseline=%d/%d, want %d/%d", scale, txt, d.Size.Y, d.Baseline, reference.Size.Y, reference.Baseline)
			}
		}
	}
}

func TestMenuTextBaseline(t *testing.T) {
	th := NewTheme()
	for _, scale := range []float32{1, 1.5, 2} {
		baseline := -1
		for _, txt := range []string{"文件", "编辑", "查看", "网关"} {
			var ops op.Ops
			gtx := C{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(56*scale), int(28*scale)))}
			d := layout.Center.Layout(gtx, th.label(txt, textSize, colInk).Layout)
			b := d.Size.Y - d.Baseline
			if baseline < 0 {
				baseline = b
			}
			if b != baseline {
				t.Errorf("scale=%g %q baseline=%d, want %d", scale, txt, b, baseline)
			}
		}
	}
}

func TestMonospaceValuesAndFixedHeight(t *testing.T) {
	th := NewTheme()
	for _, scale := range []float32{1, 1.5, 2} {
		metric := unit.Metric{PxPerDp: scale, PxPerSp: scale}
		var reference D
		for i, txt := range []string{"000000", "111111", "ABCDEF", "0x0368"} {
			var ops op.Ops
			gtx := C{Ops: &ops, Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1000, 100)}}
			d := th.mono(txt, colBody).Layout(gtx)
			if i == 0 {
				reference = d
			}
			if d != reference {
				t.Errorf("scale=%g monospace %q dimensions=%v, want %v", scale, txt, d, reference)
			}
		}
		var ops op.Ops
		gtx := C{Ops: &ops, Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1000, 100)}}
		natural := th.label("文件", textSize, colInk).Layout(gtx)
		bounded := fixedH(gtx, 32, th.label("文件", textSize, colInk).Layout)
		height := gtx.Dp(32)
		wantBaseline := natural.Size.Y - natural.Baseline + (height-natural.Size.Y)/2
		if bounded.Size.Y-bounded.Baseline != wantBaseline {
			t.Errorf("scale=%g fixed height lost baseline", scale)
		}
	}
}
