package persistence

import (
	"path/filepath"
	"testing"

	"github.com/ffutop/modbus-gateway/internal/local-slave/model"
)

// BenchmarkMemoryStorage_OnWrite benchmarks the OnWrite hook for MemoryStorage.
func BenchmarkMemoryStorage_OnWrite(b *testing.B) {
	ms := NewMemoryStorage()
	// No setup needed, OnWrite is no-op.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ms.OnWrite(model.TableHoldingRegisters, 10, 1)
	}
}

func BenchmarkFileStorage_OnWrite(b *testing.B) {
	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, "bench_file.bin")
	ms := NewFileStorage(path)
	modelPtr, err := ms.Load()
	if err != nil {
		b.Fatalf("Failed to load file storage: %v", err)
	}
	defer ms.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Modify data to be realistic
		modelPtr.HoldingRegisters[10] = uint16(i)
		ms.OnWrite(model.TableHoldingRegisters, 10, 1)
	}
}

// BenchmarkMmapStorage_OnWrite benchmarks the OnWrite hook for MmapStorage (msync).
func BenchmarkMmapStorage_OnWrite(b *testing.B) {
	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, "bench_mmap.bin")
	ms := NewMmapStorage(path)

	// We must Load() to initialize the file and mmap.
	modelPtr, err := ms.Load()
	if err != nil {
		b.Fatalf("Failed to load mmap storage: %v", err)
	}
	defer ms.Close()

	// Touch some data to ensure pages are dirty (optional, but realistic)
	modelPtr.HoldingRegisters[10] = 12345

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// We modify a value to dirty the page again (simulating real usage)
		// modifying same address repeatedly.
		modelPtr.HoldingRegisters[10] = uint16(i)
		ms.OnWrite(model.TableHoldingRegisters, 10, 1)
	}
}

// BenchmarkMemoryStorage_Load benchmarks the Load operation for MemoryStorage.
func BenchmarkMemoryStorage_Load(b *testing.B) {
	ms := NewMemoryStorage()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ms.Load()
	}
}

// BenchmarkFileStorage_Load benchmarks the Load operation for FileStorage.
func BenchmarkFileStorage_Load(b *testing.B) {
	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, "bench_file_load.bin")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ms := NewFileStorage(path)
		_, err := ms.Load()
		if err != nil {
			b.Fatalf("Load failed: %v", err)
		}
		ms.Close()
	}
}

// BenchmarkMmapStorage_Load benchmarks the Load operation for MmapStorage.
// Note: This involves file open, fstat, and mmap system calls.
func BenchmarkMmapStorage_Load(b *testing.B) {
	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, "bench_mmap_load.bin")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ms := NewMmapStorage(path)
		_, err := ms.Load()
		if err != nil {
			b.Fatalf("Load failed: %v", err)
		}
		ms.Close() // Cleanup to allow next Load
	}
}

// BenchmarkDataModel_Write benchmarks the pure in-memory write to DataModel (baseline).
func BenchmarkDataModel_Write(b *testing.B) {
	m := model.NewDataModel()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.HoldingRegisters[10] = uint16(i)
	}
}
