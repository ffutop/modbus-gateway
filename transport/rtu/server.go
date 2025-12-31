// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtu

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/transport"
	"github.com/grid-x/serial"
)

// Server implements a Modbus RTU Server (Upstream).
// It acts as a Slave on the serial bus, waiting for requests from an external Master.
type Server struct {
	Config config.SerialConfig
	Serial serialPort // Interface for testing? Or just concrete type. Using rtuSerialTransporter pattern might be better.
}

// NewServer creates a new RTU Server.
func NewServer(cfg config.SerialConfig) *Server {
	return &Server{
		Config: cfg,
	}
}

// Start starts the RTU server.
func (s *Server) Start(ctx context.Context, handler transport.RequestHandler) error {
	// 1. Open Serial Port
	// Re-using logic similar to serial.go/rtuclient.go but purely for listening.
	// Since we are Server, we just sit in a loop reading request frames.

	spConfig := &serial.Config{
		Address:  s.Config.Device,
		BaudRate: s.Config.BaudRate,
		DataBits: s.Config.DataBits,
		StopBits: s.Config.StopBits,
		Parity:   s.Config.Parity,
		Timeout:  s.Config.Timeout, // Read timeout
	}

	port, err := serial.Open(spConfig)
	if err != nil {
		return fmt.Errorf("failed to open serial port %s: %w", s.Config.Device, err)
	}
	defer port.Close()
	slog.Info("RTU Server listening", "device", s.Config.Device)

	// handle close
	go func() {
		<-ctx.Done()
		port.Close()
	}()

	// 2. Loop
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Read Request
		// We don't know the expected length or function code yet.
		// We need to read byte-by-byte or buffer until we have enough to decode.
		// However, RTU relies on time gaps (3.5 chars) to delimit frames.
		// If the underlying serial driver handles this, great. If not, we rely on read timeout.
		// `serial` package usually returns on timeout or partial read.

		// To properly implement RTU server reading, we need strict timing.
		// For now, let's reuse `readIncrementally` logic from rtuclient.go if possible,
		// BUT `readIncrementally` expects to know the Function Code beforehand (from the dispatched request).
		// Here, we are the SERVER. We don't know what's coming.
		// We need a proper "RTU Frame Scanner".

		// Simplified strategy:
		// Read 1 byte (SlaveID).
		// Read 1 byte (FunctionCode).
		// Determine expected length based on FunctionCode.
		// Read rest.

		// Wait for start of frame (SlaveID)
		buf := make([]byte, 260) // Max RTU size

		// This ReadFull is risky if we get garbage.
		// Best practice: Read 1 byte.
		n, err := port.Read(buf[:1])
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// Timeout is common when idle
			continue
		}
		if n == 0 {
			continue
		}

		slaveID := buf[0]
		_ = slaveID

		// Read PDU + CRC
		// We need at least 3 more bytes (Function + CRC_L + CRC_H)
		// But function determines length.
		n, err = port.Read(buf[1:2])
		if err != nil || n == 0 {
			continue
		}
		functionCode := buf[1]

		// Now we guess payload size.
		// If function > 0x80, it's an error? No, master sends requests.
		// Master never sends error codes.

		// Standard Modbus Request Lengths are fixed or determined by byte count.
		// expectedLen := calculateRequestLength(functionCode, buf)
		_ = calculateRequestLength(functionCode, buf)
		// Note: calculateRequestLength needs more data for some codes (like WriteMultiple encoded in header).
		// So we might need to read more.

		// Hard part: RTU Server implementation needs a robust state machine.
		// For this iteration, assuming standard well-behaved frames for simplicity,
		// relying on timeout to delimit frame end is safer.
		// Implementation: Read untill timeout? No, that's slow.

		// Let's implement a "Read available until silence" or simple length parser.
		// Let's reuse the logic from `readIncrementally` but adapt it for Server (Request).
		// Actually, `readIncrementally` is specifically for Response.
		// Requests have different structure (e.g. Write Multiple Registers has a byte count).

		// Let's try to read the whole frame incrementally based on parsed length.

		// ... (Implementation detail handling) ...

		// Just reading what we can for now to unblock.
		// Correct approach:
		// Read header, parse length, read rest.

		_ = slaveID

	}
}

// Helper to determine request length
func calculateRequestLength(funcCode byte, partial []byte) int {
	// Minimal implementation
	// TODO: Full implementation
	return 8 // generic placeholder
}

func (s *Server) Close() error {
	return nil
}
