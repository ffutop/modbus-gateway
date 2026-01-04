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
	"github.com/ffutop/modbus-gateway/modbus/crc"
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
	return s.scanLoop(ctx, port, handler)
}

func (s *Server) scanLoop(ctx context.Context, port io.ReadWriteCloser, handler transport.RequestHandler) error {
	buf := make([]byte, 260) // Max RTU size

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
		expectedLen, err := calculateRequestLength(functionCode, buf[:current])
		if err != nil {
			// slog.Debug("Invalid request or partial read", "err", err)
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

		// Verify CRC
		var c crc.CRC
		c.Reset().PushBytes(buf[:expectedLen-2])
		checksum := c.Value()
		receivedChecksum := uint16(buf[expectedLen-1])<<8 | uint16(buf[expectedLen-2])

		if checksum != receivedChecksum {
			// CRC Mismatch
			continue
		}

		// Extract PDU
		slaveID := buf[0]
		pduData := make([]byte, expectedLen-3)
		copy(pduData, buf[1:expectedLen-2])

		// pduData[0] is Code, pduData[1:] is Data
		reqPDU := modbus.ProtocolDataUnit{
			FunctionCode: functionCode,
			Data:         pduData[1:],
		}

		// Dispatch
		go func(sid byte, pdu modbus.ProtocolDataUnit) {
			respPDU, err := handler(ctx, sid, pdu)
			if err != nil {
				slog.Error("Upstream handler failed", "err", err)
				return
			}

			// Construct Response ADU
			// [SlaveID] [Func] [Data] [CRC]
			respLen := 1 + 1 + len(respPDU.Data) + 2
			respBuf := make([]byte, respLen)
			respBuf[0] = sid
			respBuf[1] = respPDU.FunctionCode
			copy(respBuf[2:], respPDU.Data)

			// CRC
			var c crc.CRC
			c.Reset().PushBytes(respBuf[:respLen-2])
			sum := c.Value()
			respBuf[respLen-1] = byte(sum >> 8)
			respBuf[respLen-2] = byte(sum)

			_, _ = port.Write(respBuf)

		}(slaveID, reqPDU)
	}
}

// calculateRequestLength returns the expected total length of the RTU ADU
func calculateRequestLength(funcCode byte, header []byte) (int, error) {
	// Header should be at least 7 bytes to cover ByteCount for 0x0F/0x10.
	// [SlaveID, Func, Appd1, Appd2, Appd3, Appd4/ByteCount]

	switch funcCode {
	case 0x01, 0x02, 0x03, 0x04, 0x05, 0x06:
		// Fixed 8 bytes: [SlaveID, Func, Addr(2), Val(2), CRC(2)]
		return 8, nil
	case 0x0F, 0x10:
		// Write Multiple
		// Req: [SlaveID, Func, Addr(2), Quant(2), ByteCount(1), Data(N), CRC(2)]
		// ByteCount is at Offset 6 (0-indexed) = header[6]

		if len(header) < 7 {
			return 0, fmt.Errorf("need 7 bytes to determine length for 0x%02X, got %d", funcCode, len(header))
		}

		byteCount := int(header[6])
		// Total = 7 (Header up to ByteCount) + N (Data) + 2 (CRC)
		return 7 + byteCount + 2, nil
	default:
		// Assume unknown function codes are not supported or have fixed minimal length?
		// For robustness, discard.
		return 0, fmt.Errorf("unsupported function code: 0x%02X", funcCode)
	}
}

func (s *Server) Close() error {
	return nil
}
