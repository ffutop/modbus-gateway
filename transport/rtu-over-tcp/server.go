// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtuovertcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/ffutop/modbus-gateway/modbus"
	rtupacket "github.com/ffutop/modbus-gateway/modbus/rtu"
	"github.com/ffutop/modbus-gateway/transport"
)

// Server implements a Modbus RTU over TCP Server.
// It listens on a TCP port and handles incoming connections as Modbus RTU streams.
type Server struct {
	Address  string
	listener net.Listener
}

// NewServer creates a new RTU over TCP Server.
func NewServer(address string) *Server {
	return &Server{
		Address: address,
	}
}

// Start starts the TCP server.
func (s *Server) Start(ctx context.Context, handler transport.RequestHandler) error {
	listener, err := net.Listen("tcp", s.Address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.Address, err)
	}
	s.listener = listener
	slog.Info("RTU over TCP server listening", "addr", s.Address)

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				slog.Error("Failed to accept connection", "err", err)
				continue
			}
		}
		go s.handleConnection(ctx, conn, handler)
	}
}

// Close closes the server listener.
func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn, handler transport.RequestHandler) {
	defer conn.Close()
	slog.Info("New RTU over TCP client connected", "addr", conn.RemoteAddr())

	// Buffer for reading (reusing max size from RTU package)
	buf := make([]byte, rtupacket.MaxSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 1. Read first byte (SlaveID) to detect start of frame
		// We limit read to 1 byte to strictly control the stream consumption
		n, err := conn.Read(buf[:1])
		if err != nil {
			if err != io.EOF {
				slog.Error("Connection read error", "addr", conn.RemoteAddr(), "err", err)
			}
			return
		}
		if n == 0 {
			continue
		}

		// 2. Read enough header bytes to determine frame length.
		// We need at least 7 bytes total (including SlaveID) for some commands (like 0x10)
		// to contain the ByteCount field.
		current := 1
		need := 7

		for current < need {
			n, err := conn.Read(buf[current:need])
			if err != nil {
				return // Stop on error
			}
			current += n
		}

		// 3. Determine expected length
		functionCode := buf[1]
		expectedLen, err := rtupacket.CalculateRequestLength(functionCode, buf[:current])
		if err != nil {
			slog.Warn("Invalid RTU frame header", "func", functionCode, "err", err)
			// Strategy: Close connection on protocol violation to reset stream state
			// or try to skip? Closing is safer for RTU over TCP.
			return
		}

		// 4. Read remaining body
		for current < expectedLen {
			n, err := conn.Read(buf[current:expectedLen])
			if err != nil {
				return
			}
			current += n
		}

		// 5. Decode and Verify CRC
		adu, err := rtupacket.Decode(buf[:expectedLen])
		if err != nil {
			slog.Warn("RTU frame decode failed", "err", err)
			continue
		}

		// 6. Handle Request
		respPdu, err := handler(ctx, adu.SlaveID, adu.Pdu)
		if err != nil {
			slog.Error("Handler failed", "err", err)
			// Map error to Modbus exception code
			exceptionCode := modbus.ExceptionCodeServerDeviceFailure
			if errors.Is(err, context.DeadlineExceeded) {
				exceptionCode = modbus.ExceptionCodeGatewayTargetDeviceFailedToRespond
			}
			// Construct Exception PDU: Function Code | 0x80
			respPdu = modbus.ProtocolDataUnit{
				FunctionCode: adu.Pdu.FunctionCode | 0x80,
				Data:         []byte{byte(exceptionCode)},
			}
		}

		// 7. Send Response
		respAdu := &rtupacket.ApplicationDataUnit{
			SlaveID: adu.SlaveID,
			Pdu:     respPdu,
		}

		respRaw, err := respAdu.Encode()
		if err != nil {
			slog.Error("Failed to encode response", "err", err)
			continue
		}

		if _, err := conn.Write(respRaw); err != nil {
			slog.Error("Failed to write response", "err", err)
			return
		}
	}
}