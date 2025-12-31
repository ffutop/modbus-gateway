// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package tcp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/ffutop/modbus-gateway/transport"
)

// Server implements a Modbus TCP Server.
type Server struct {
	Address string
	Handler transport.RequestHandler

	listener net.Listener
}

// NewServer creates a new TCP Server.
func NewServer(address string) *Server {
	return &Server{
		Address: address,
	}
}

// Start starts the TCP server.
func (s *Server) Start(ctx context.Context, handler transport.RequestHandler) error {
	s.Handler = handler
	listener, err := net.Listen("tcp", s.Address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.Address, err)
	}
	s.listener = listener
	slog.Info("Modbus TCP server listening", "addr", s.Address)

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Check if closed
			select {
			case <-ctx.Done():
				return nil
			default:
				slog.Error("Failed to accept connection", "err", err)
				continue
			}
		}
		go s.handleConnection(ctx, conn)
	}
}

// Close closes the server listener.
func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	slog.Info("New TCP client connected", "addr", conn.RemoteAddr())

	for {
		// Check context
		select {
		case <-ctx.Done():
			return
		default:
		}

		// max MODBUS TCP ADU = 260 bytes.
		buf := make([]byte, 260+1) // +1 to detect overflow
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				slog.Info("TCP client disconnected gracefully", "addr", conn.RemoteAddr())
			} else {
				slog.Error("Failed to read from connection", "addr", conn.RemoteAddr(), "err", err)
			}
			return
		}

		if n > 260 {
			slog.Error("Invalid request length", "length", n)
			return
		}

		adu, err := Decode(buf[:n])
		if err != nil {
			slog.Error("Failed to decode TCP request", "err", err)
			continue
		}

		if s.Handler == nil {
			slog.Error("No handler defined for TCP server")
			return
		}

		respPdu, err := s.Handler(ctx, adu.SlaveID, adu.Pdu)
		if err != nil {
			slog.Error("Handler failed", "err", err)
			// TODO: Send exception response?
			continue
		}

		// Construct Response ADU
		respAdu := &ApplicationDataUnit{
			TransactionID: adu.TransactionID,
			ProtocolID:    adu.ProtocolID,
			Length:        uint16(1 + 1 + len(respPdu.Data)), // SlaveID + FunctionCode + Data
			SlaveID:       adu.SlaveID,
			Pdu:           respPdu,
		}

		respRaw, err := respAdu.Encode()
		if err != nil {
			slog.Error("Failed to encode TCP response", "err", err)
			continue
		}

		_, err = conn.Write(respRaw)
		if err != nil {
			slog.Error("Failed to write response to connection", "err", err)
			return
		}
	}
}
