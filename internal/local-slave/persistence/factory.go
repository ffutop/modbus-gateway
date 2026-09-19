// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package persistence

import "log/slog"

// Config is the minimal persistence configuration New needs. It mirrors
// config.PersistenceConfig without importing the config package (which
// would create a dependency cycle, since config never needs to know about
// persistence implementations).
type Config struct {
	Type string
	Path string
}

// New builds a Storage implementation from a persistence type name.
// Unknown/empty types default to MemoryStorage (non-persistent), matching
// today's behavior.
func New(cfg Config) Storage {
	switch cfg.Type {
	case "file":
		slog.Info("Initializing simulation with file persistence", "path", cfg.Path)
		return NewFileStorage(cfg.Path)
	case "mmap":
		slog.Info("Initializing simulation with mmap persistence", "path", cfg.Path)
		return NewMmapStorage(cfg.Path)
	case "sql":
		slog.Info("Initializing simulation with SQL persistence", "driver", "sqlite3", "dsn", cfg.Path)
		return NewSQLStorage("sqlite3", cfg.Path)
	default:
		slog.Info("Initializing simulation with memory storage (non-persistent)")
		return NewMemoryStorage()
	}
}
