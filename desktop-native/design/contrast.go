// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package design

import (
	"math"
	"strconv"
)

// ContrastRule is a foreground/background pair of semantic colors that must
// reach Min (WCAG 2 contrast ratio). The tests and the token studio both
// check these rules.
type ContrastRule struct {
	Fg  string  `json:"fg"`
	Bg  string  `json:"bg"`
	Min float64 `json:"min"`
}

// ContrastRules: text roles need AA (4.5:1) on every surface they sit on;
// indicators and focus rings need 3:1 against the canvas. warn-solid is
// exempt: amber dots always sit next to a text label (see README.md).
var ContrastRules = []ContrastRule{
	{"ink", "canvas", 4.5}, {"body", "canvas", 4.5}, {"muted", "canvas", 4.5}, {"muted", "soft", 4.5},
	{"ok", "ok-bg", 4.5}, {"ok", "canvas", 4.5},
	{"warn", "warn-bg", 4.5}, {"warn", "canvas", 4.5},
	{"err", "err-bg", 4.5}, {"err", "canvas", 4.5}, {"err", "err-tint", 4.5},
	{"on-dark", "ink", 4.5}, {"on-dark-body", "dark", 4.5}, {"on-dark-muted", "dark", 4.5},
	{"on-primary", "primary", 4.5}, {"on-selected", "selected", 4.5},
	{"ok-solid", "canvas", 3}, {"err-solid", "canvas", 3}, {"accent", "canvas", 3}, {"primary", "canvas", 3},
}

// Contrast is the WCAG 2 contrast ratio between two #rrggbb colors.
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	var rgb [3]float64
	for i := range rgb {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		x := float64(v) / 255
		if x <= 0.03928 {
			rgb[i] = x / 12.92
		} else {
			rgb[i] = math.Pow((x+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*rgb[0] + 0.7152*rgb[1] + 0.0722*rgb[2]
}
