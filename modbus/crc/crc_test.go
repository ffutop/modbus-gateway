// Copyright (c) 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package crc

import (
	"testing"
)

func TestCRC(t *testing.T) {
	var crc CRC
	crc.Reset()
	crc.PushBytes([]byte{0x02, 0x07})

	if crc.Value() != 0x1241 {
		t.Fatalf("crc expected %v, actual %v", 0x1241, crc.Value())
	}
}
