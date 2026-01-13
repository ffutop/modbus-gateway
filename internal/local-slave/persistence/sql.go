// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package persistence

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
)

// SQLStorage implements persistence using a SQL database.
// It assumes a table `modbus_registers` exists (or creates it).
type SQLStorage struct {
	driver string
	dsn    string
	db     *sql.DB
	model  *model.DataModel
}

// NewSQLStorage creates a new SQLStorage.
// Note: The driver (e.g., sqlite3, mysql) must be imported in main.go
func NewSQLStorage(driver, dsn string) *SQLStorage {
	return &SQLStorage{
		driver: driver,
		dsn:    dsn,
	}
}

// Load connects to the DB and loads the data.
func (s *SQLStorage) Load() (*model.DataModel, error) {
	db, err := sql.Open(s.driver, s.dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}
	s.db = db

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	m := model.NewDataModel()
	s.model = m // Keep reference for OnWrite logic if needed (though we have values)

	// Load data from DB
	rows, err := db.Query("SELECT table_type, address, value FROM modbus_registers")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to query registers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t int
		var addr, val int
		if err := rows.Scan(&t, &addr, &val); err != nil {
			continue
		}
		if addr > model.MaxAddress {
			continue
		}

		switch model.TableType(t) {
		case model.TableCoils:
			m.Coils[addr] = byte(val)
		case model.TableDiscreteInputs:
			m.DiscreteInputs[addr] = byte(val)
		case model.TableHoldingRegisters:
			m.HoldingRegisters[addr] = uint16(val)
		case model.TableInputRegisters:
			m.InputRegisters[addr] = uint16(val)
		}
	}

	return m, nil
}

func (s *SQLStorage) initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS modbus_registers (
		table_type INTEGER,
		address INTEGER,
		value INTEGER,
		PRIMARY KEY (table_type, address)
	);
	`
	_, err := s.db.Exec(query)
	return err
}

// Save is a full save. For SQL, we might not want to do this often.
// But if requested, we upsert everything? That's too heavy.
// We assume OnWrite handles real-time sync.
// Save() might be used for snapshotting, but for DB it's redundant if OnWrite works.
// We implement it as a no-op or full dump (optional).
func (s *SQLStorage) Save(m *model.DataModel) error {
	// Full save is expensive and typically not needed if OnWrite is reliable.
	return nil
}

// OnWrite upserts the changed register to the DB.
func (s *SQLStorage) OnWrite(table model.TableType, address, quantity uint16) {
	if s.db == nil || s.model == nil {
		return
	}

	// We need to read the new values from the model to write them.
	// Since OnWrite is called AFTER model update, we can read s.model.
	// To avoid blocking the caller too long, we might want to do this async,
	// but "real-time persistence" implies safety.
	// We'll do it synchronously for now or fire a goroutine?
	// The prompt implies "real-time persistence" to prevent data loss on power failure.
	// Sync write is safer.

	// Use a transaction for the batch?
	// But `quantity` is usually small (1 or a few).
	// If quantity is large, batch insert is better.

	for i := 0; i < int(quantity); i++ {
		addr := int(address) + i
		var val int64

		switch table {
		case model.TableCoils:
			val = int64(s.model.Coils[addr])
		case model.TableDiscreteInputs:
			val = int64(s.model.DiscreteInputs[addr])
		case model.TableHoldingRegisters:
			val = int64(s.model.HoldingRegisters[addr])
		case model.TableInputRegisters:
			val = int64(s.model.InputRegisters[addr])
		}

		// Upsert logic (SQLite compatible)
		// "INSERT OR REPLACE" or "ON CONFLICT"
		query := "INSERT INTO modbus_registers (table_type, address, value) VALUES (?, ?, ?) ON CONFLICT(table_type, address) DO UPDATE SET value=excluded.value"
		_, err := s.db.Exec(query, int(table), addr, val)
		if err != nil {
			slog.Error("Failed to persist register", "table", table, "addr", addr, "err", err)
		}
	}
}

func (s *SQLStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
