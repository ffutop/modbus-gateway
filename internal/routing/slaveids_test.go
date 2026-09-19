// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package routing

import (
	"reflect"
	"testing"
)

func TestParseSlaveIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []byte
		wantErr bool
	}{
		{name: "single", input: "1", want: []byte{1}},
		{name: "list", input: "1,2,5", want: []byte{1, 2, 5}},
		{name: "range", input: "1-3", want: []byte{1, 2, 3}},
		{name: "mixed", input: "1, 3-5, 10", want: []byte{1, 3, 4, 5, 10}},
		{name: "empty", input: "", want: nil},
		{name: "invalid range order", input: "5-1", wantErr: true},
		{name: "invalid id", input: "abc", wantErr: true},
		{name: "out of range", input: "300", wantErr: true},
		{name: "malformed range", input: "1-2-3", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSlaveIDs(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
