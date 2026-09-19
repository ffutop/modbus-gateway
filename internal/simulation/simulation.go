// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

// Package simulation implements the shared simulated data model contract:
// a named DataModel plus its persistence, referenced by any number of
// business ("local") and injection ("injector") downstreams.
package simulation

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
	"github.com/ffutop/modbus-gateway/internal/local-slave/persistence"
)

// Status describes the operational state of a Simulation.
type Status string

const (
	// StatusReady means the model is serving requests and persistence is healthy.
	StatusReady Status = "ready"
	// StatusDegraded means the model is still serving requests, but a runtime
	// persistence failure occurred and has been recorded in the audit log.
	// Degraded is sticky: it does not auto-recover.
	StatusDegraded Status = "degraded"
)

// Simulation wraps a shared DataModel with its persistence and status
// bookkeeping. It is the single point through which every write to the
// model - whether from a business "local" downstream or an "injector"
// downstream - is committed, persisted and audited.
type Simulation struct {
	Name  string
	Model *model.DataModel

	storage persistence.Storage

	mu       sync.Mutex
	version  uint64
	degraded bool
}

// Open loads (or creates) the model for name via storage. Open must succeed
// before any downstream referencing this simulation is allowed to start.
func Open(name string, storage persistence.Storage) (*Simulation, error) {
	m, err := storage.Load()
	if err != nil {
		return nil, fmt.Errorf("simulation %q: failed to open persistence: %w", name, err)
	}
	return &Simulation{
		Name:    name,
		Model:   m,
		storage: storage,
	}, nil
}

// Status returns the current operational status.
func (s *Simulation) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.degraded {
		return StatusDegraded
	}
	return StatusReady
}

// Version returns the monotonically increasing commit version. It is bumped
// once per successful write, regardless of whether persistence subsequently
// succeeds.
func (s *Simulation) Version() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

// Close releases the underlying persistence resources.
func (s *Simulation) Close() error {
	if closer, ok := s.storage.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// commit records a successful in-memory write: it bumps the version and
// triggers the persistence hook. A persistence failure marks the simulation
// degraded and is audited, but never fails the caller - the in-memory write
// already succeeded and is visible to readers.
func (s *Simulation) commit(table model.TableType, address, quantity uint16) {
	s.mu.Lock()
	s.version++
	s.mu.Unlock()

	if err := s.storage.OnWrite(table, address, quantity); err != nil {
		s.markDegraded(err)
	}
}

func (s *Simulation) markDegraded(err error) {
	s.mu.Lock()
	wasDegraded := s.degraded
	s.degraded = true
	s.mu.Unlock()

	if !wasDegraded {
		slog.Error("simulation persistence degraded", "simulation", s.Name, "err", err)
	}
}

// WriteSingleCoil writes a single coil (business Modbus write path).
func (s *Simulation) WriteSingleCoil(address, value uint16) error {
	if err := s.Model.WriteSingleCoil(address, value); err != nil {
		return err
	}
	s.commit(model.TableCoils, address, 1)
	return nil
}

// WriteMultipleCoils writes a range of coils (business Modbus write path).
func (s *Simulation) WriteMultipleCoils(address, quantity uint16, data []byte) error {
	if err := s.Model.WriteMultipleCoils(address, quantity, data); err != nil {
		return err
	}
	s.commit(model.TableCoils, address, quantity)
	return nil
}

// WriteSingleRegister writes a single holding register (business Modbus write path).
func (s *Simulation) WriteSingleRegister(address, value uint16) error {
	if err := s.Model.WriteSingleRegister(address, value); err != nil {
		return err
	}
	s.commit(model.TableHoldingRegisters, address, 1)
	return nil
}

// WriteMultipleRegisters writes a range of holding registers (business Modbus write path).
func (s *Simulation) WriteMultipleRegisters(address, quantity uint16, data []byte) error {
	if err := s.Model.WriteMultipleRegisters(address, quantity, data); err != nil {
		return err
	}
	s.commit(model.TableHoldingRegisters, address, quantity)
	return nil
}

// WriteSingleDiscreteInput writes a single discrete input (injector write path).
func (s *Simulation) WriteSingleDiscreteInput(address, value uint16) error {
	if err := s.Model.WriteSingleDiscreteInput(address, value); err != nil {
		return err
	}
	s.commit(model.TableDiscreteInputs, address, 1)
	return nil
}

// WriteMultipleDiscreteInputs writes a range of discrete inputs (injector write path).
func (s *Simulation) WriteMultipleDiscreteInputs(address, quantity uint16, data []byte) error {
	if err := s.Model.WriteMultipleDiscreteInputs(address, quantity, data); err != nil {
		return err
	}
	s.commit(model.TableDiscreteInputs, address, quantity)
	return nil
}

// WriteSingleInputRegister writes a single input register (injector write path).
func (s *Simulation) WriteSingleInputRegister(address, value uint16) error {
	if err := s.Model.WriteSingleInputRegister(address, value); err != nil {
		return err
	}
	s.commit(model.TableInputRegisters, address, 1)
	return nil
}

// WriteMultipleInputRegisters writes a range of input registers (injector write path).
func (s *Simulation) WriteMultipleInputRegisters(address, quantity uint16, data []byte) error {
	if err := s.Model.WriteMultipleInputRegisters(address, quantity, data); err != nil {
		return err
	}
	s.commit(model.TableInputRegisters, address, quantity)
	return nil
}
