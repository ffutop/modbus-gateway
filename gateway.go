package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/ffutop/modbus-gateway/transport"
	"github.com/ffutop/modbus-gateway/transport/rtu"
	"github.com/ffutop/modbus-gateway/transport/tcp"
)

type queuedRequest struct {
	request  *tcp.ApplicationDataUnit
	response chan<- *queuedResponse
}

type queuedResponse struct {
	response []byte
	err      error
}

type Gateway struct {
	config      *Config
	rtuClient   *transport.Client
	requestChan chan *queuedRequest
}

func NewGateway(config *Config) *Gateway {
	return &Gateway{
		config: config,
		// init queue, set a reasonable buffer size, e.g. 100
		requestChan: make(chan *queuedRequest, 100),
	}
}

// Run Gateway
func (g *Gateway) Run() error {
	slog.Info("init Modbus RTU client", "device", g.config.Device, "baudRate", g.config.BaudRate, "dataBits", g.config.DataBits, "parity", g.config.Parity, "stopBits", g.config.StopBits)
	// Create a RTU Client Handler
	rtuHandler := rtu.NewRTUClientHandler(g.config.Device)
	rtuHandler.BaudRate = g.config.BaudRate
	rtuHandler.DataBits = g.config.DataBits
	rtuHandler.Parity = g.config.Parity
	rtuHandler.StopBits = g.config.StopBits
	rtuHandler.Timeout = g.config.Timeout

	// Create a RTU Client
	g.rtuClient = transport.NewClient(rtuHandler)

	// Connect to RTU Device
	err := g.rtuClient.Connect(context.Background())
	if err != nil {
		return err
	}
	defer g.rtuClient.Close()

	// Start a background worker to process RTU requests serially
	go g.rtuWorker()

	// Use native net library to build TCP server
	addr := fmt.Sprintf("%s:%d", g.config.TCPAddress, g.config.TCPPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	defer listener.Close()
	slog.Info("init paired Modbus TCP server", "addr", addr, "device", g.config.Device)

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Error("Failed to accept connection", "err", err)
			continue
		}
		go g.handleConnection(conn)
	}
}

// handleConnection, handle a single TCP client connection.
func (g *Gateway) handleConnection(conn net.Conn) {
	defer conn.Close()
	slog.Info("New TCP client connected", "addr", conn.RemoteAddr())

	for {
		// max MODBUS RS232/RS485 ADU = 253 bytes + Server address (1 byte) + CRC (2 bytes) = 256 bytes
		// max MODBUS TCP ADU = 253 bytes + MBAP (7 bytes) = 260 bytes.
		buf := make([]byte, 261)
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				slog.Info("TCP client disconnected gracefully", "addr", conn.RemoteAddr())
			} else {
				slog.Error("Failed to read from connection", "addr", conn.RemoteAddr(), "err", err)
			}
			return
		}

		if n == 261 {
			slog.Error("Invalid request length", "length", n)
			return
		}

		adu, err := tcp.Decode(buf[:n])
		if err != nil {
			slog.Error("Failed to decode TCP request", "err", err)
			continue
		}
		responseChan := make(chan *queuedResponse)
		queuedRequest := &queuedRequest{request: adu, response: responseChan}
		g.requestChan <- queuedRequest
		result := <-responseChan
		if result.err != nil {
			slog.Error("Failed to process request", "err", result.err)
			continue
		}
		// write response to TCP client
		_, err = conn.Write(result.response)
		if err != nil {
			slog.Error("Failed to write response to connection", "err", err)
			return
		}
	}
}

// rtuWorker is a background goroutine that processes RTU requests serially.
// It receives requests from the requestChan and sends them to the RTU device
func (g *Gateway) rtuWorker() {
	slog.Debug("RTU worker started", "device", g.config.Device)
	for req := range g.requestChan {
		tcpAduReq := req.request

		// transform TCP ADU to RTU ADU
		rtuAduReq := &rtu.ApplicationDataUnit{
			SlaveID: tcpAduReq.SlaveID,
			Pdu:     tcpAduReq.Pdu,
		}
		rtuRawReq, err := rtuAduReq.Encode()
		if err != nil {
			slog.Error("Failed to encode RTU request", "err", err)
			continue
		}

		// Send request to RTU device
		rtuRawResp, err := g.rtuClient.Send(context.Background(), rtuRawReq)
		if err != nil {
			slog.Error("RTU request failed", "err", err)
		} else {
			slog.Info("RTU response received", "response", hex.EncodeToString(rtuRawResp))
		}

		// transform RTU ADU to TCP ADU
		rtuAduResp, err := rtu.Decode(rtuRawResp)
		if err != nil {
			slog.Error("Failed to decode RTU response", "err", err)
			continue
		}
		tcpAduResp := &tcp.ApplicationDataUnit{
			TransactionID: req.request.TransactionID,
			ProtocolID:    req.request.ProtocolID,
			Length:        uint16(len(rtuAduResp.Pdu.Data) + 2),
			SlaveID:       req.request.SlaveID,
			Pdu:           rtuAduResp.Pdu,
		}

		tcpRawResp, err := tcpAduResp.Encode()
		if err != nil {
			slog.Error("Failed to encode TCP response", "err", err)
			continue
		}
		slog.Info("TCP response encoded", "response", hex.EncodeToString(tcpRawResp))

		// Send response back to the original request goroutine
		req.response <- &queuedResponse{response: tcpRawResp, err: err}
	}
	slog.Debug("RTU worker stopped", "device", g.config.Device)
}
