// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package design

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func load(t *testing.T) *Tokens {
	t.Helper()
	tok, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// root is the repository root; tests run in the design package directory.
func root(rel string) string {
	return filepath.Join("..", filepath.FromSlash(rel))
}

// TestGeneratedUpToDate fails when a generated file no longer matches
// tokens.json, i.e. someone edited tokens.json without regenerating or
// edited a generated file by hand.
func TestGeneratedUpToDate(t *testing.T) {
	for path, want := range load(t).Render() {
		got, err := os.ReadFile(root(path))
		if os.IsNotExist(err) {
			if _, derr := os.Stat(filepath.Dir(root(path))); os.IsNotExist(derr) {
				continue // module not present in this checkout
			}
		}
		if err != nil {
			t.Errorf("%s: %v (run `go generate ./design`)", path, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale; run `go generate ./design`", path)
		}
	}
}

// Raw color literals outside the token files bypass the design system.
var (
	cssColor = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\brgba?\(`)
	goColor  = regexp.MustCompile(`\brgb\(0x[0-9a-fA-F]+\)|color\.NRGBA\{R:\s*0x`)
)

// TestNoRawColors keeps every color in the product UIs flowing from the tokens.
func TestNoRawColors(t *testing.T) {
	files := []string{
		"web/app.css", "web/app.js", "web/index.html",
		"desktop/stopped.css", "desktop/stopped.html", "desktop/stopped.js",
	}
	native, _ := filepath.Glob(root("desktop-native/internal/ui/*.go"))
	for _, f := range native {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, "tokens_gen.go") {
			continue
		}
		files = append(files, "desktop-native/internal/ui/"+filepath.Base(f))
	}
	for _, rel := range files {
		data, err := os.ReadFile(root(rel))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		re := cssColor
		if strings.HasSuffix(rel, ".go") {
			re = goColor
		}
		for i, line := range strings.Split(string(data), "\n") {
			if m := re.FindString(line); m != "" {
				t.Errorf("%s:%d: raw color %q; use a token from design/tokens.json", rel, i+1, m)
			}
		}
	}
}

func TestContrastRules(t *testing.T) {
	tok := load(t)
	for _, r := range ContrastRules {
		fg, _ := tok.Hex(r.Fg)
		bg, _ := tok.Hex(r.Bg)
		if c := Contrast(fg, bg); c < r.Min {
			t.Errorf("%s on %s: contrast %.2f < %v", r.Fg, r.Bg, c, r.Min)
		}
	}
}

// TestFileRoundTrip keeps saves from the token studio diff-stable: tokens.json
// is always in the canonical layout that File produces.
func TestFileRoundTrip(t *testing.T) {
	if got := load(t).File(); !bytes.Equal(got, tokensJSON) {
		t.Errorf("tokens.json is not in canonical layout; run `go generate ./design`")
	}
}
