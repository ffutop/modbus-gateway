// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package design

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Source is the token source file, relative to the repository root.
const Source = "design/tokens.json"

// File returns t as tokens.json in its canonical layout: one entry per line,
// so a change to one token is a one-line diff.
func (t *Tokens) File() []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	field(&b, 1, "$comment", t.Comment, ",\n")
	list(&b, "palette", len(t.Palette), func(i int) []kv {
		p := t.Palette[i]
		return []kv{{"name", p.Name}, {"value", p.Value}, {"use", p.Use}}
	})
	list(&b, "color", len(t.Color), func(i int) []kv {
		c := t.Color[i]
		return []kv{{"name", c.Name}, {"ref", c.Ref}, {"use", c.Use}}
	})
	list(&b, "alpha", len(t.Alpha), func(i int) []kv {
		a := t.Alpha[i]
		return []kv{{"name", a.Name}, {"value", a.Value}, {"use", a.Use}}
	})
	for _, g := range []struct {
		key  string
		dims []Dimension
	}{{"fontSize", t.FontSize}, {"radius", t.Radius}} {
		dims := g.dims
		list(&b, g.key, len(dims), func(i int) []kv {
			return []kv{{"name", dims[i].Name}, {"value", dims[i].Value}, {"use", dims[i].Use}}
		})
	}
	b.WriteString("  \"density\": {\n")
	field(&b, 2, "$comment", t.Density.Comment, ",\n")
	field(&b, 2, "desktop-native", t.Density.DesktopNative, "\n")
	b.WriteString("  }\n}\n")
	return b.Bytes()
}

type kv struct {
	k string
	v interface{}
}

func list(b *bytes.Buffer, key string, n int, entry func(int) []kv) {
	b.WriteString("  " + quote(key) + ": [\n")
	for i := 0; i < n; i++ {
		b.WriteString("    {")
		first := true
		for _, f := range entry(i) {
			if s, ok := f.v.(string); ok && s == "" && f.k == "use" {
				continue
			}
			if !first {
				b.WriteString(",")
			}
			first = false
			b.WriteString(" " + quote(f.k) + ": " + quote(f.v))
		}
		b.WriteString(" }")
		if i < n-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  ],\n")
}

func field(b *bytes.Buffer, depth int, key string, v interface{}, end string) {
	if s, ok := v.(string); ok && s == "" {
		return
	}
	b.WriteString(string(bytes.Repeat([]byte("  "), depth)) + quote(key) + ": " + quote(v) + end)
}

// quote encodes v as JSON without HTML escaping, so font stacks stay readable.
func quote(v interface{}) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return string(bytes.TrimSuffix(b.Bytes(), []byte("\n")))
}

// WriteFiles writes tokens.json in canonical layout plus every generated file
// under root, skipping targets whose directory is absent (a module missing
// from a partial checkout). It returns the paths it wrote, sorted.
func (t *Tokens) WriteFiles(root string) ([]string, error) {
	out := t.Render()
	out[Source] = t.File()
	paths := make([]string, 0, len(out))
	for p := range out {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var wrote []string
	for _, p := range paths {
		dst := filepath.Join(root, filepath.FromSlash(p))
		if _, err := os.Stat(filepath.Dir(dst)); os.IsNotExist(err) {
			continue
		}
		if err := writeAtomic(dst, out[p]); err != nil {
			return wrote, err
		}
		wrote = append(wrote, p)
	}
	return wrote, nil
}

// writeAtomic replaces dst without leaving a half-written file behind.
func writeAtomic(dst string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}
