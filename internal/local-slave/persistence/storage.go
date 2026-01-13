// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package persistence

import (
	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
)

// Storage defines the interface for persisting the local slave data model.
type Storage interface {
	// Load loads the data model from storage.
	// If no data exists, it should return a new empty model (or nil error and caller handles it).
	Load() (*model.DataModel, error)

	// Save saves the current data model to storage.
	Save(model *model.DataModel) error

	// OnWrite is a hook called whenever a register is modified.
	// It allows the storage to perform real-time persistence (e.g. sync to disk or DB).
	OnWrite(table model.TableType, address, quantity uint16)
}
