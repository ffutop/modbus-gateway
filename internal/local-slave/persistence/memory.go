// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package persistence

import "github.com/ffutop/modbus-gateway/internal/local-slave/model"

// MemoryStorage is a no-op storage (non-persistent).
type MemoryStorage struct{}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{}
}

func (ms *MemoryStorage) Load() (*model.DataModel, error) {
	return model.NewDataModel(), nil
}

func (ms *MemoryStorage) Save(model *model.DataModel) error {
	return nil
}

func (ms *MemoryStorage) OnWrite(table model.TableType, address, quantity uint16) {
	// No-op
}
