package uifont

import (
	"gioui.org/font"
	"testing"
)

func TestBundledNotoCoverage(t *testing.T) {
	weights := map[font.Weight]bool{}
	for _, f := range Collection() {
		if f.Font.Typeface != "Noto Sans SC" {
			continue
		}
		weights[f.Font.Weight] = true
		face := f.Face.Face()
		for _, r := range "文件编辑查看网关锅炉房寄存器中文注释—…→" {
			if _, ok := face.NominalGlyph(r); !ok {
				t.Errorf("Noto weight %v lacks %q", f.Font.Weight, r)
			}
		}
	}
	if !weights[font.Normal] || !weights[font.Bold] {
		t.Fatalf("missing bundled Noto regular/bold: %v", weights)
	}
}
