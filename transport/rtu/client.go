// Copyright (c) 2014 Quoc-Viet Nguyen. All rights reserved.
// Copyright (c) 2025 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtu

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/ffutop/modbus-gateway/internal/config"
	"github.com/ffutop/modbus-gateway/modbus"
	rtupacket "github.com/ffutop/modbus-gateway/modbus/rtu"
)

// Client implements Downstream interface (Modbus RTU Master).
type Client struct {
	rtuSerialTransporter
}

// NewClient allocates and initializes a RTU Client.
func NewClient(cfg config.SerialConfig) *Client {
	client := &Client{}

	// Map internal config to serial.Config
	client.serialPort.Config.Address = cfg.Device
	client.serialPort.Config.BaudRate = cfg.BaudRate
	client.serialPort.Config.DataBits = cfg.DataBits
	client.serialPort.Config.StopBits = cfg.StopBits
	client.serialPort.Config.Parity = cfg.Parity
	client.serialPort.Config.Timeout = cfg.Timeout

	client.IdleTimeout = serialIdleTimeout
	return client
}

// Send sends a PDU to the Downstream Slave
func (mb *Client) Send(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	// Wrap PDU into RTU ADU
	adu := &rtupacket.ApplicationDataUnit{
		SlaveID: slaveID,
		Pdu:     pdu,
	}

	aduBytes, err := adu.Encode()
	if err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to encode ADU: %w", err)
	}

	// Send via Serial
	respBytes, err := mb.rtuSerialTransporter.Send(ctx, aduBytes)
	if err != nil {
		return modbus.ProtocolDataUnit{}, err
	}

	// Decode Response
	respAdu, err := rtupacket.Decode(respBytes)
	if err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("failed to decode response ADU: %w", err)
	}

	// Verify
	if err := adu.Verify(respAdu); err != nil {
		return modbus.ProtocolDataUnit{}, fmt.Errorf("verification failed: %w", err)
	}

	return respAdu.Pdu, nil
}

// rtuSerialTransporter implements underlying serial comms.
type rtuSerialTransporter struct {
	serialPort
}

func (mb *rtuSerialTransporter) Send(ctx context.Context, aduRequest []byte) (aduResponse []byte, err error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if err = mb.connect(ctx); err != nil {
		return
	}
	mb.lastActivity = time.Now()
	mb.startCloseTimer()

	slog.Debug("send to modbus slave", "request", hex.EncodeToString(aduRequest))
	if _, err = mb.port.Write(aduRequest); err != nil {
		return
	}

	bytesToRead := rtupacket.CalculateResponseLength(aduRequest)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(mb.calculateDelay(len(aduRequest) + bytesToRead)):
	}

	data, err := rtupacket.ReadResponse(aduRequest[0], aduRequest[1], mb.port, time.Now().Add(mb.Config.Timeout))
	if err != nil {
		return nil, err
	}
	slog.Debug("recv from modbus slave", "response", hex.EncodeToString(data[:]))
	aduResponse = data
	return
}

// calculateDelay calculates the needed delay to separate frames.
func (mb *rtuSerialTransporter) calculateDelay(chars int) time.Duration {
	var characterDelay, frameDelay int

	if mb.BaudRate <= 0 || mb.BaudRate > 19200 {
		characterDelay = 750
		frameDelay = 1750
	} else {
		characterDelay = 15000000 / mb.BaudRate
		frameDelay = 35000000 / mb.BaudRate
	}
	return time.Duration(characterDelay*chars+frameDelay) * time.Microsecond
}