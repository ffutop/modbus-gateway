// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package routing holds slave-ID routing helpers shared by the config
// loader (for validation) and the gateway (for building routing tables).
package routing

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSlaveIDs parses a string of slave IDs (e.g. "1,2,5-10") into a slice of bytes.
func ParseSlaveIDs(input string) ([]byte, error) {
	var ids []byte
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			// Range
			ranges := strings.Split(part, "-")
			if len(ranges) != 2 {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			start, err := strconv.Atoi(strings.TrimSpace(ranges[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start of range: %w", err)
			}
			end, err := strconv.Atoi(strings.TrimSpace(ranges[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end of range: %w", err)
			}
			if start > end {
				return nil, fmt.Errorf("start of range %d is greater than end %d", start, end)
			}
			for i := start; i <= end; i++ {
				if i < 0 || i > 255 {
					return nil, fmt.Errorf("id out of range: %d", i)
				}
				ids = append(ids, byte(i))
			}
		} else {
			// Single
			id, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid id: %w", err)
			}
			if id < 0 || id > 255 {
				return nil, fmt.Errorf("id out of range: %d", id)
			}
			ids = append(ids, byte(id))
		}
	}
	return ids, nil
}
