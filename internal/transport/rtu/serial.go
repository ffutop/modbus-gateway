// Copyright (c) 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package rtu

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/grid-x/serial"
)

const (
	// Default timeout
	serialTimeout     = 5 * time.Second
	serialIdleTimeout = 60 * time.Second
)

// serialPort has configuration and I/O controller.
type serialPort struct {
	// Serial port configuration.
	serial.Config

	IdleTimeout time.Duration

	mu sync.Mutex
	// port is platform-dependent data structure for serial port.
	port         io.ReadWriteCloser
	lastActivity time.Time
	closeTimer   *time.Timer
}

func (modbus *serialPort) Connect(ctx context.Context) (err error) {
	modbus.mu.Lock()
	defer modbus.mu.Unlock()

	return modbus.connect(ctx)
}

// connect connects to the serial port if it is not connected. Caller must hold the mutex.
func (modbus *serialPort) connect(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if modbus.port == nil {
		port, err := serial.Open(&modbus.Config)
		if err != nil {
			return fmt.Errorf("could not open %s: %w", modbus.Config.Address, err)
		}
		modbus.port = port
	}
	return nil
}

func (modbus *serialPort) Close() (err error) {
	modbus.mu.Lock()
	defer modbus.mu.Unlock()

	return modbus.close()
}

// close closes the serial port if it is connected. Caller must hold the mutex.
func (modbus *serialPort) close() (err error) {
	if modbus.port != nil {
		err = modbus.port.Close()
		modbus.port = nil
	}
	return
}

func (modbus *serialPort) logf(format string, v ...interface{}) {
	slog.Debug(format, v...)
}

func (modbus *serialPort) startCloseTimer() {
	if modbus.IdleTimeout <= 0 {
		return
	}
	if modbus.closeTimer == nil {
		modbus.closeTimer = time.AfterFunc(modbus.IdleTimeout, modbus.closeIdle)
	} else {
		modbus.closeTimer.Reset(modbus.IdleTimeout)
	}
}

// closeIdle closes the connection if last activity is passed behind IdleTimeout.
func (modbus *serialPort) closeIdle() {
	modbus.mu.Lock()
	defer modbus.mu.Unlock()

	if modbus.IdleTimeout <= 0 {
		return
	}

	if idle := time.Since(modbus.lastActivity); idle >= modbus.IdleTimeout {
		modbus.logf("modbus: closing connection due to idle timeout: %v", idle)
		modbus.close()
	}
}
