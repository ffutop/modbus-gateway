// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package simulation

import (
	"errors"
	"testing"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
)

// fakeStorage is a controllable persistence.Storage for testing Simulation's
// degraded/version bookkeeping without touching disk.
type fakeStorage struct {
	loadErr    error
	onWriteErr error
	onWriteN   int
}

func (f *fakeStorage) Load() (*model.DataModel, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return model.NewDataModel(), nil
}

func (f *fakeStorage) Save(m *model.DataModel) error { return nil }

func (f *fakeStorage) OnWrite(table model.TableType, address, quantity uint16) error {
	f.onWriteN++
	return f.onWriteErr
}

func TestOpen_PropagatesLoadError(t *testing.T) {
	fs := &fakeStorage{loadErr: errors.New("disk full")}
	if _, err := Open("sim", fs); err == nil {
		t.Fatal("expected error from Open when storage.Load fails")
	}
}

func TestOpen_StartsReady(t *testing.T) {
	sim, err := Open("sim", &fakeStorage{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sim.Status() != StatusReady {
		t.Errorf("expected status %q, got %q", StatusReady, sim.Status())
	}
	if sim.Version() != 0 {
		t.Errorf("expected version 0, got %d", sim.Version())
	}
}

func TestWrite_SuccessBumpsVersionAndStaysReady(t *testing.T) {
	fs := &fakeStorage{}
	sim, err := Open("sim", fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := sim.WriteSingleCoil(0, 0xFF00); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sim.Version() != 1 {
		t.Errorf("expected version 1, got %d", sim.Version())
	}
	if sim.Status() != StatusReady {
		t.Errorf("expected status %q, got %q", StatusReady, sim.Status())
	}
	if fs.onWriteN != 1 {
		t.Errorf("expected OnWrite to be called once, got %d", fs.onWriteN)
	}

	data, err := sim.Model.ReadCoils(0, 1)
	if err != nil || data[0] != 1 {
		t.Errorf("expected coil 0 to be set, got %v err=%v", data, err)
	}
}

func TestWrite_PersistenceFailure_StillSucceedsButDegraded(t *testing.T) {
	fs := &fakeStorage{onWriteErr: errors.New("disk full")}
	sim, err := Open("sim", fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := sim.WriteSingleCoil(0, 0xFF00); err != nil {
		t.Fatalf("expected write to succeed despite persistence failure, got %v", err)
	}
	if sim.Status() != StatusDegraded {
		t.Errorf("expected status %q after persistence failure, got %q", StatusDegraded, sim.Status())
	}

	// The in-memory model must reflect the write even though persistence failed.
	data, err := sim.Model.ReadCoils(0, 1)
	if err != nil || data[0] != 1 {
		t.Errorf("expected coil 0 to be set despite degraded persistence, got %v err=%v", data, err)
	}

	// Degraded is sticky: subsequent successful in-memory writes keep succeeding
	// and the model does not auto-recover to ready.
	fs.onWriteErr = nil
	if err := sim.WriteSingleCoil(1, 0xFF00); err != nil {
		t.Fatalf("expected write to keep succeeding while degraded, got %v", err)
	}
	if sim.Status() != StatusDegraded {
		t.Errorf("expected status to remain %q, got %q", StatusDegraded, sim.Status())
	}
}

func TestWrite_ModelValidationErrorIsReturnedAndNotPersisted(t *testing.T) {
	fs := &fakeStorage{}
	sim, err := Open("sim", fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// quantity 0 is invalid at the model layer.
	if err := sim.WriteMultipleCoils(0, 0, nil); err == nil {
		t.Fatal("expected validation error from invalid write")
	}
	if fs.onWriteN != 0 {
		t.Errorf("expected OnWrite not to be called for a rejected write, got %d calls", fs.onWriteN)
	}
	if sim.Version() != 0 {
		t.Errorf("expected version to stay 0 for a rejected write, got %d", sim.Version())
	}
}

func TestInjectionWrites_DiscreteInputsAndInputRegisters(t *testing.T) {
	sim, err := Open("sim", &fakeStorage{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := sim.WriteSingleDiscreteInput(3, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := sim.WriteMultipleInputRegisters(0, 1, []byte{0x12, 0x34}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sim.Version() != 2 {
		t.Errorf("expected version 2, got %d", sim.Version())
	}

	di, _ := sim.Model.ReadDiscreteInputs(3, 1)
	if di[0] != 1 {
		t.Errorf("expected discrete input 3 to be set")
	}
	ir, _ := sim.Model.ReadInputRegisters(0, 1)
	if ir[0] != 0x12 || ir[1] != 0x34 {
		t.Errorf("expected input register 0 to be 0x1234, got %x", ir)
	}
}
