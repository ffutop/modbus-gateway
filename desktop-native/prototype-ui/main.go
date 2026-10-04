// PROTOTYPE — throwaway. Delete once a layout is chosen (see NOTES.md).
//
// Round three: the chosen layout (C1 with the gateway topology above the
// linked view) and configuration editing in two modes, visual and raw
// config.yaml, over simulated traffic.
//
//	cd desktop-native && go run ./prototype-ui
package main

import (
	"flag"
	"fmt"
	"image"
	"os"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

type variant interface {
	Name() string
	Layout(gtx C) D
}

func main() {
	shots := flag.String("snapshot", "", "render every variant to PNGs in this directory and exit")
	shotSize := flag.String("snapshot-size", "1440x900", "logical snapshot viewport, WIDTHxHEIGHT")
	applyFailure := flag.Bool("apply-failure", false, "simulate a failed first application; retry succeeds")
	flag.Parse()
	if *shots != "" {
		width, height := 1440, 900
		if _, err := fmt.Sscanf(*shotSize, "%dx%d", &width, &height); err != nil || width < 1100 || height < 680 {
			panic("snapshot-size must be WIDTHxHEIGHT, at least 1100x680")
		}
		if err := snapshotSize(*shots, width, height); err != nil {
			panic(err)
		}
		return
	}

	w := new(app.Window)
	w.Option(app.Title("ModMux 原型"), app.Size(unit.Dp(1440), unit.Dp(900)), app.MinSize(unit.Dp(1100), unit.Dp(680)))

	world := newWorld()
	stop := make(chan struct{})
	go world.run(stop, w.Invalidate)

	th := NewTheme()
	cfg := newConfigEditor(th, world)
	cfg.simulateFailure = *applyFailure
	variants := []variant{newVariantC1(th, world, cfg)}
	sw := &switcher{th: th, variants: variants}

	go func() {
		var ops op.Ops
		for {
			switch e := w.Event().(type) {
			case app.DestroyEvent:
				close(stop)
				os.Exit(0)
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				sw.Layout(gtx)
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}

// switcher shows the current variant with a floating bar to flip between
// them. It is the prototype's chrome, not part of any design.
type switcher struct {
	th         *Theme
	variants   []variant
	cur        int
	prev, next widget.Clickable
}

func (s *switcher) Layout(gtx C) D {
	n := len(s.variants)
	if s.prev.Clicked(gtx) {
		s.cur = (s.cur + n - 1) % n
	}
	if s.next.Clicked(gtx) {
		s.cur = (s.cur + 1) % n
	}
	// ←/→ reach us only when no focused editor takes them.
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameLeftArrow}, key.Filter{Name: key.NameRightArrow})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			if ke.Name == key.NameLeftArrow {
				s.cur = (s.cur + n - 1) % n
			} else {
				s.cur = (s.cur + 1) % n
			}
		}
	}

	paint.Fill(gtx.Ops, colCanvas)
	if n == 1 {
		return s.variants[0].Layout(gtx)
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(s.variants[s.cur].Layout),
		layout.Stacked(func(gtx C) D {
			gtx.Constraints.Min = gtx.Constraints.Max
			return layout.S.Layout(gtx, func(gtx C) D {
				return layout.Inset{Bottom: 14}.Layout(gtx, s.bar)
			})
		}),
	)
}

func (s *switcher) bar(gtx C) D {
	th := s.th
	key := fmt.Sprintf("C%d", s.cur+1)
	arrow := func(btn *widget.Clickable, txt string) layout.FlexChild {
		return layout.Rigid(func(gtx C) D {
			return btn.Layout(gtx, func(gtx C) D {
				return layout.Inset{Left: 10, Right: 10, Top: 6, Bottom: 6}.Layout(gtx, th.bold(txt, 15, colCanvas).Layout)
			})
		})
	}
	gtx.Constraints.Min = image.Point{}
	return rounded(gtx, alpha(colDark, 235), 18, func(gtx C) D {
		return layout.Inset{Left: 4, Right: 4}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				arrow(&s.prev, "‹"),
				layout.Rigid(th.bold(key+" — "+s.variants[s.cur].Name(), textSize, colCanvas).Layout),
				gap(8),
				layout.Rigid(th.label("原型 · ←/→ 切换", smallSize, alpha(colCanvas, 150)).Layout),
				arrow(&s.next, "›"),
			)
		})
	})
}
