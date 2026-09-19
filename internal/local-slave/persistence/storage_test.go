// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package persistence

import (
	"path/filepath"
	"testing"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
)

func TestMemoryStorage_OnWrite_NeverFails(t *testing.T) {
	ms := NewMemoryStorage()
	if err := ms.OnWrite(model.TableHoldingRegisters, 0, 1); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestFileStorage_OnWrite_Succeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file_storage.bin")
	ms := NewFileStorage(path)
	if _, err := ms.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer ms.Close()

	if err := ms.OnWrite(model.TableHoldingRegisters, 0, 1); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestFileStorage_OnWrite_FailsWhenNotLoaded(t *testing.T) {
	ms := NewFileStorage(filepath.Join(t.TempDir(), "unloaded.bin"))
	// Load() was never called, so ms.file/ms.data are nil.
	if err := ms.OnWrite(model.TableHoldingRegisters, 0, 1); err == nil {
		t.Error("expected error when OnWrite is called before Load")
	}
}

func TestMmapStorage_OnWrite_FailsWhenNotLoaded(t *testing.T) {
	ms := NewMmapStorage(filepath.Join(t.TempDir(), "unloaded.bin"))
	if err := ms.OnWrite(model.TableHoldingRegisters, 0, 1); err == nil {
		t.Error("expected error when OnWrite is called before Load")
	}
}

func TestMmapStorage_OnWrite_Succeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mmap_storage.bin")
	ms := NewMmapStorage(path)
	if _, err := ms.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer ms.Close()

	if err := ms.OnWrite(model.TableHoldingRegisters, 0, 1); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestSQLStorage_OnWrite_FailsWhenNotLoaded(t *testing.T) {
	ms := NewSQLStorage("sqlite3", filepath.Join(t.TempDir(), "unloaded.db"))
	// Load() was never called, so ms.db/ms.model are nil.
	if err := ms.OnWrite(model.TableHoldingRegisters, 0, 1); err == nil {
		t.Error("expected error when OnWrite is called before Load")
	}
}
