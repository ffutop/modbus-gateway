// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtu

import "testing"

func TestCalculateRequestLength(t *testing.T) {
	tests := []struct {
		name     string
		funcCode byte
		header   []byte
		want     int
		wantErr  bool
	}{
		{"ReadHoldingRegisters", 0x03, []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01}, 8, false},
		{"WriteSingleRegister", 0x06, []byte{0x01, 0x06, 0x00, 0x00, 0xAA, 0xBB}, 8, false},
		{"WriteMultipleRegisters_ShortHeader", 0x10, []byte{0x01, 0x10, 0x00, 0x01, 0x00, 0x01}, 0, true},
		{"WriteMultipleRegisters_Valid", 0x10, []byte{0x01, 0x10, 0x00, 0x01, 0x00, 0x01, 0x02}, 7 + 2 + 2, false},
		{"UnknownFunction", 0x99, []byte{0x01, 0x99}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateRequestLength(tt.funcCode, tt.header)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateRequestLength() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("calculateRequestLength() = %v, want %v", got, tt.want)
			}
		})
	}
}
