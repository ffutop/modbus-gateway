package filepicker

import (
	"testing"
	"unsafe"
)

func TestOpenFileNameLayout(t *testing.T) {
	// sizeof(OPENFILENAMEW) on 64-bit Windows.
	if unsafe.Sizeof(uintptr(0)) == 8 && unsafe.Sizeof(openFileName{}) != 152 {
		t.Fatalf("OPENFILENAMEW size %d", unsafe.Sizeof(openFileName{}))
	}
	f := filter("A", "*.a")
	if len(f) != 7 || f[1] != 0 || f[5] != 0 || f[6] != 0 {
		t.Fatalf("filter %v", f)
	}
}
