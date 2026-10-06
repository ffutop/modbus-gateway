package workspace

import (
	"gioui.org/f32"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"image"
	"image/color"
)

// Role icons share a 16 dp view box. Arrows always follow request flow.
func configRoleIcon(gtx C, role string, ink color.NRGBA) D {
	size := gtx.Dp(16)
	scale := float32(size) / 16
	var p clip.Path
	p.Begin(gtx.Ops)
	line := func(points ...f32.Point) {
		p.MoveTo(points[0].Mul(scale))
		for _, v := range points[1:] {
			p.LineTo(v.Mul(scale))
		}
	}
	switch role {
	case "总览":
		for _, x := range []float32{2, 10} {
			for _, y := range []float32{2, 10} {
				line(f32.Pt(x, y), f32.Pt(x+4, y), f32.Pt(x+4, y+4), f32.Pt(x, y+4), f32.Pt(x, y))
			}
		}
	case "上游", "下游":
		line(f32.Pt(6, 4), f32.Pt(10, 4), f32.Pt(10, 12), f32.Pt(6, 12), f32.Pt(6, 4))
		line(f32.Pt(0, 8), f32.Pt(6, 8))
		line(f32.Pt(10, 8), f32.Pt(16, 8))
		if role == "上游" {
			line(f32.Pt(3, 5), f32.Pt(6, 8), f32.Pt(3, 11))
		} else {
			line(f32.Pt(13, 5), f32.Pt(16, 8), f32.Pt(13, 11))
		}
	case "模拟模型":
		line(f32.Pt(1, 5), f32.Pt(8, 1), f32.Pt(15, 5), f32.Pt(8, 9), f32.Pt(1, 5))
		line(f32.Pt(1, 8), f32.Pt(8, 12), f32.Pt(15, 8))
		line(f32.Pt(1, 11), f32.Pt(8, 15), f32.Pt(15, 11))
	case "常规":
		for i, x := range []float32{3, 8, 13} {
			y := float32(4 + i*3)
			line(f32.Pt(x, 1), f32.Pt(x, y-2))
			line(f32.Pt(x, y+2), f32.Pt(x, 15))
			line(f32.Pt(x-2, y-2), f32.Pt(x+2, y-2), f32.Pt(x+2, y+2), f32.Pt(x-2, y+2), f32.Pt(x-2, y-2))
		}
	default:
		line(f32.Pt(2, 4), f32.Pt(14, 4), f32.Pt(14, 12), f32.Pt(2, 12), f32.Pt(2, 4))
		line(f32.Pt(0, 8), f32.Pt(2, 8))
		line(f32.Pt(14, 8), f32.Pt(16, 8))
		line(f32.Pt(5, 8), f32.Pt(11, 8))
	}
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: p.End(), Width: float32(gtx.Dp(1))}.Op())
	return D{Size: image.Pt(size, size)}
}
