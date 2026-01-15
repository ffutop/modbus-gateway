// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtu

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/modbus"
	rtupacket "github.com/ffutop/modbus-gateway/modbus/rtu"
	"github.com/ffutop/modbus-gateway/transport"
	"github.com/grid-x/serial"
)

// Server implements a Modbus RTU Server (Upstream).
// It acts as a Slave on the serial bus, waiting for requests from an external Master.
type Server struct {
	Config config.SerialConfig
	Serial serialPort
}

// NewServer creates a new RTU Server.
func NewServer(cfg config.SerialConfig) *Server {
	return &Server{
		Config: cfg,
	}
}

// Start starts the RTU server.
func (s *Server) Start(ctx context.Context, handler transport.RequestHandler) error {
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

	go func() {
		<-ctx.Done()
		port.Close()
	}()

	return s.scanLoop(ctx, port, handler)
}

func (s *Server) scanLoop(ctx context.Context, port io.ReadWriteCloser, handler transport.RequestHandler) error {
	buf := make([]byte, rtupacket.MaxSize)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Robust Frame Scanning
		// Read 1 byte to unblock
		n, err := port.Read(buf[:1])
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		if n == 0 {
			continue
		}

		// Read header (attempt 7 bytes total to cover ByteCount for variable length functions)
		current := 1
		need := 7

		for current < need {
			n, err := port.Read(buf[current:need])
			if err != nil {
				break
			}
			current += n
		}

		if current < 2 {
			continue
		}

		functionCode := buf[1]

		// Determine expected length
		expectedLen, err := rtupacket.CalculateRequestLength(functionCode, buf[:current])
		if err != nil {
			continue
		}

		// Read remaining
		for current < expectedLen {
			n, err := port.Read(buf[current:expectedLen])
			if err != nil {
				break
			}
			current += n
		}

		if current != expectedLen {
			continue
		}

		// Decode ADU (Verifies CRC and structure)
		adu, err := rtupacket.Decode(buf[:expectedLen])
		if err != nil {
			// CRC Mismatch or invalid packet
			continue
		}

		// Dispatch
		go func(sid byte, pdu modbus.ProtocolDataUnit) {
			respPDU, err := handler(ctx, sid, pdu)
			if err != nil {
				slog.Error("Upstream handler failed", "err", err)
				return
			}

			// Construct Response ADU
			respAdu := &rtupacket.ApplicationDataUnit{
				SlaveID: sid,
				Pdu:     respPDU,
			}

			respBuf, err := respAdu.Encode()
			if err != nil {
				slog.Error("Failed to encode response ADU", "err", err)
				return
			}

			_, _ = port.Write(respBuf)

		}(adu.SlaveID, adu.Pdu)
	}
}

func (s *Server) Close() error {
	return nil
}