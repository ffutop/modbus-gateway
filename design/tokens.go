// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package design renders the design tokens in tokens.json into the Go
// declarations the Gio desktop app consumes.
package design

//go:generate go run ./cmd/gentokens

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/format"
	"math"
	"regexp"
	"strconv"
	"strings"
)

//go:embed tokens.json
var tokensJSON []byte

// Tokens mirrors tokens.json. Lists keep their order so the generated files
// read in the same order as the source.
type Tokens struct {
	Comment  string      `json:"$comment"`
	Palette  []Value     `json:"palette"`
	Color    []Ref       `json:"color"`
	Alpha    []Value     `json:"alpha"`
	FontSize []Dimension `json:"fontSize"`
	Radius   []Dimension `json:"radius"`
	Density  struct {
		Comment       string  `json:"$comment"`
		DesktopNative float64 `json:"desktop-native"`
	} `json:"density"`
}

type Value struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Use   string `json:"use"`
}

type Ref struct {
	Name string `json:"name"`
	Ref  string `json:"ref"`
	Use  string `json:"use"`
}

type Dimension struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Use   string  `json:"use"`
}

var (
	nameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	hexRe  = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	rgbaRe = regexp.MustCompile(`^rgba\((\d{1,3}),(\d{1,3}),(\d{1,3}),(0|1|0?\.\d+)\)$`)
)

// rgba parses an alpha token value such as "rgba(0,0,0,.33)" into its
// color and its alpha byte.
func rgba(value string) (rgb uint32, a uint8, ok bool) {
	m := rgbaRe.FindStringSubmatch(value)
	if m == nil {
		return 0, 0, false
	}
	for _, c := range m[1:4] {
		v, _ := strconv.Atoi(c)
		if v > 255 {
			return 0, 0, false
		}
		rgb = rgb<<8 | uint32(v)
	}
	f, _ := strconv.ParseFloat(m[4], 64)
	return rgb, uint8(math.Round(f * 255)), true
}

// Load parses and validates the embedded tokens.json.
func Load() (*Tokens, error) { return Parse(tokensJSON) }

// Parse parses and validates a tokens.json document.
func Parse(data []byte) (*Tokens, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var t Tokens
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("tokens.json: %w", err)
	}
	return &t, t.validate()
}

func (t *Tokens) validate() error {
	palette := map[string]bool{}
	for _, p := range t.Palette {
		if !nameRe.MatchString(p.Name) || !hexRe.MatchString(p.Value) {
			return fmt.Errorf("palette %q: want a kebab-case name and a lowercase #rrggbb value, got %q", p.Name, p.Value)
		}
		if palette[p.Name] {
			return fmt.Errorf("palette %q: duplicate", p.Name)
		}
		palette[p.Name] = true
	}
	used := map[string]bool{}
	seen := map[string]bool{}
	check := func(group, name string) error {
		if !nameRe.MatchString(name) {
			return fmt.Errorf("%s %q: name must be kebab-case", group, name)
		}
		if seen[group+"/"+name] {
			return fmt.Errorf("%s %q: duplicate", group, name)
		}
		seen[group+"/"+name] = true
		return nil
	}
	for _, c := range t.Color {
		if err := check("color", c.Name); err != nil {
			return err
		}
		if !palette[c.Ref] {
			return fmt.Errorf("color %q: unknown palette entry %q", c.Name, c.Ref)
		}
		used[c.Ref] = true
	}
	for _, p := range t.Palette {
		if !used[p.Name] {
			return fmt.Errorf("palette %q: not referenced by any color; remove it", p.Name)
		}
	}
	for _, v := range t.Alpha {
		if err := check("alpha", v.Name); err != nil {
			return err
		}
		if _, _, ok := rgba(v.Value); !ok {
			return fmt.Errorf("alpha %q: want rgba(r,g,b,a), got %q", v.Name, v.Value)
		}
	}
	for _, group := range []struct {
		name string
		dims []Dimension
	}{{"fontSize", t.FontSize}, {"radius", t.Radius}} {
		for _, d := range group.dims {
			if err := check(group.name, d.Name); err != nil {
				return err
			}
			if d.Value <= 0 {
				return fmt.Errorf("%s %q: value must be positive", group.name, d.Name)
			}
		}
	}
	if t.Density.DesktopNative <= 0 {
		return fmt.Errorf("density: desktop-native must be positive")
	}
	for _, r := range ContrastRules {
		for _, name := range []string{r.Fg, r.Bg} {
			if !seen["color/"+name] {
				return fmt.Errorf("color %q: required by the contrast rules", name)
			}
		}
	}
	return nil
}

// Hex returns the #rrggbb value of a semantic color.
func (t *Tokens) Hex(color string) (string, bool) {
	for _, c := range t.Color {
		if c.Name == color {
			return t.hex(c.Ref), true
		}
	}
	return "", false
}

func (t *Tokens) hex(ref string) string {
	for _, p := range t.Palette {
		if p.Name == ref {
			return p.Value
		}
	}
	panic("unvalidated palette ref " + ref)
}

// DesktopNative is the generated file's path, relative to the repository root.
const DesktopNative = "desktop-native/internal/workspace/tokens_gen.go"

// Render returns every generated file, keyed by its path relative to the
// repository root.
func (t *Tokens) Render() map[string][]byte {
	return map[string][]byte{DesktopNative: t.gio()}
}

const header = "Code generated by design/cmd/gentokens from design/tokens.json; DO NOT EDIT."

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// camel turns "ok-bg" into "OkBg".
func camel(name string) string {
	parts := strings.Split(name, "-")
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

func (t *Tokens) gio() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// %s\n\npackage workspace\n\nimport \"gioui.org/unit\"\n\n", header)
	b.WriteString("// Colors, by semantic role.\nvar (\n")
	for _, c := range t.Color {
		fmt.Fprintf(&b, "\tcol%s = rgb(0x%s)\n", camel(c.Name), strings.TrimPrefix(t.hex(c.Ref), "#"))
	}
	b.WriteString(")\n\n// Translucent overlays.\nvar (\n")
	for _, v := range t.Alpha {
		c, a, _ := rgba(v.Value)
		fmt.Fprintf(&b, "\tcol%s = alpha(rgb(0x%06x), %d)\n", camel(v.Name), c, a)
	}
	b.WriteString(")\n\n")
	fmt.Fprintf(&b, "// Font sizes: design size x density %s, rounded to 0.5sp.\nconst (\n", num(t.Density.DesktopNative))
	for _, d := range t.FontSize {
		fmt.Fprintf(&b, "\tfs%s unit.Sp = %s\n", camel(d.Name), num(math.Round(d.Value*t.Density.DesktopNative*2)/2))
	}
	b.WriteString(")\n\n// Corner radii.\nconst (\n")
	for _, d := range t.Radius {
		fmt.Fprintf(&b, "\tradius%s unit.Dp = %s\n", camel(d.Name), num(d.Value))
	}
	b.WriteString(")\n")
	src, err := format.Source(b.Bytes())
	if err != nil {
		panic(fmt.Sprintf("generated Go does not parse: %v", err))
	}
	return src
}
