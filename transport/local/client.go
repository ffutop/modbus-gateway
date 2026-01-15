// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package local

import (
	"context"
	"log/slog"

	"github.com/ffutop/modbus-gateway/internal/config"
	localslave "github.com/ffutop/modbus-gateway/internal/local-slave"
	"github.com/ffutop/modbus-gateway/internal/local-slave/persistence"
	"github.com/ffutop/modbus-gateway/modbus"
)

// Client implements Downstream interface for a local in-memory slave.
type Client struct {
	slave   *localslave.LocalSlave
	storage persistence.Storage
}

// NewClient creates a new Local Client.
func NewClient(cfg config.LocalConfig) *Client {
	var storage persistence.Storage
	switch cfg.Persistence.Type {
	case "file":
		slog.Info("Initializing local slave with file persistence", "path", cfg.Persistence.Path)
		storage = persistence.NewFileStorage(cfg.Persistence.Path)
	case "mmap":
		slog.Info("Initializing local slave with MMAP persistence", "path", cfg.Persistence.Path)
		storage = persistence.NewMmapStorage(cfg.Persistence.Path)
	case "sql":
		slog.Info("Initializing local slave with SQL persistence", "driver", "sqlite3", "dsn", cfg.Persistence.Path)
		// Assuming Path contains DSN for now, or we need a new config field.
		// Re-using Path as DSN is simple.
		// Note: The main app must import the driver (e.g. _ "github.com/mattn/go-sqlite3")
		storage = persistence.NewSQLStorage("sqlite3", cfg.Persistence.Path)
	default:
		slog.Info("Initializing local slave with memory storage (non-persistent)")
		storage = persistence.NewMemoryStorage()
	}

	m, err := storage.Load()
	if err != nil {
		slog.Error("Failed to load persistence data, starting with fresh model", "err", err)
		// If mmap fails, we probably shouldn't continue or we fall back to memory
		if m == nil {
			slog.Warn("Falling back to MemoryStorage")
			storage = persistence.NewMemoryStorage()
			m, _ = storage.Load()
		}
	}

	// Initialize protocol logic
	s := localslave.NewLocalSlave(m, storage)

	return &Client{
		slave:   s,
		storage: storage,
	}
}

// Send processes the PDU locally.
func (c *Client) Send(ctx context.Context, slaveID byte, pdu modbus.ProtocolDataUnit) (modbus.ProtocolDataUnit, error) {
	// The LocalSlave is synchronous and fast, so we just call Process.
	return c.slave.Process(pdu)
}

// Connect is a no-op for local slave.
func (c *Client) Connect(ctx context.Context) error {
	return nil
}

// Close closes the storage.
func (c *Client) Close() error {
	if closer, ok := c.storage.(interface{ Close() }); ok {
		closer.Close()
	}
	return nil
}
