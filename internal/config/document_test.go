// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func readDoc(t *testing.T, content string) *Document {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	d, err := ReadDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDocumentSet_RewritesOnlyTheValueText(t *testing.T) {
	tests := []struct {
		name, in string
		path     []any
		value    any
		want     string
	}{
		{"plain with trailing comment", "a: old   # keep me\n", []any{"a"}, "new", "a: new   # keep me\n"},
		{"double quoted keeps quotes", "a: \"100\" # id\n", []any{"a"}, "101", "a: \"101\" # id\n"},
		{"single quoted with escape", "a: 'it''s'\nb: 1\n", []any{"a"}, "o'clock", "a: 'o''clock'\nb: 1\n"},
		{"inside a flow map", "t: { ref: line-a, x: 1 }\n", []any{"t", "ref"}, "line-b", "t: { ref: line-b, x: 1 }\n"},
		{"flow value needing quotes", "t: { ref: a }\n", []any{"t", "ref"}, "a,b", "t: { ref: \"a,b\" }\n"},
		{"plain value needing quotes", "a: x\n", []any{"a"}, "1:2 # no", "a: '1:2 # no'\n"},
		{"number", "s:\n  - n: 5\n", []any{"s", 0, "n"}, 7, "s:\n  - n: 7\n"},
		{"wide characters earlier on the line", "名称: 旧值 # 注释\n", []any{"名称"}, "新值", "名称: 新值 # 注释\n"},
		{"sequence index from JSON", "s: [a, b]\n", []any{"s", float64(1)}, "c", "s: [a, c]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := readDoc(t, tt.in)
			if err := d.Apply([]Edit{{Op: "set", Path: tt.path, Value: tt.value}}); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			got := d.Bytes()
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDocumentSet_RefusesWhatItCannotRewriteSafely(t *testing.T) {
	const in = "a: 1\nm:\n  k: v\nblock: |\n  line one\n  line two\n"
	for name, e := range map[string]Edit{
		"missing key":        {Op: "set", Path: []any{"nope"}, Value: 1},
		"mapping target":     {Op: "set", Path: []any{"m"}, Value: "x"},
		"block scalar":       {Op: "set", Path: []any{"block"}, Value: "x"},
		"multi-line value":   {Op: "set", Path: []any{"a"}, Value: "x\ny"},
		"unknown op":         {Op: "delete", Path: []any{"a"}},
		"index into mapping": {Op: "set", Path: []any{"m", 0}, Value: 1},
	} {
		t.Run(name, func(t *testing.T) {
			d := readDoc(t, in)
			if err := d.Apply([]Edit{e}); err == nil {
				t.Error("Apply succeeded, want an error")
			}
			if got := d.Bytes(); string(got) != in {
				t.Errorf("content changed to %q", got)
			}
		})
	}
}
