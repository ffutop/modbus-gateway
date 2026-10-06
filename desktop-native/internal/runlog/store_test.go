package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestStructuredRecordsAndSessionRetention(t *testing.T) {
	s := New(nil)
	first := s.BeginSession("first.yaml")
	s.Append("gateway", first, `{"time":"2026-10-05T12:00:00Z","level":"ERROR","msg":"Connection failed","gateway":"line-a","err":"timeout"}`)
	s.Append("gateway", first, `{"event":"ui_ready","addr":"127.0.0.1:1234"}`)
	s.Record("desktop", 0, "INFO", "配置已保存", "path", "new.yaml")
	second := s.BeginSession("second.yaml")
	s.Append("gateway", second, "unstructured startup failure")
	b := s.Snapshot(0)
	if len(b.Entries) != 5 || b.Last != 5 {
		t.Fatalf("snapshot: %+v", b)
	}
	e := b.Entries[1]
	if e.Gateway != "line-a" || e.Level != "ERROR" || e.Session != first || e.Message != "Connection failed" || !strings.Contains(e.Fields, "timeout") {
		t.Fatalf("record: %+v", e)
	}
	if b.Entries[4].Level != "UNKNOWN" || first == second {
		t.Fatal("raw output or session identity lost")
	}
	if len(s.Snapshot(3).Entries) != 2 {
		t.Fatal("watermark failed")
	}
}

func TestBoundsAndConcurrentWriters(t *testing.T) {
	s := New(nil)
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1500; i++ {
				s.Append("desktop", 0, fmt.Sprint(i))
			}
		}()
	}
	wg.Wait()
	b := s.Snapshot(0)
	if len(b.Entries) != Capacity || b.Dropped != 1000 || b.Last != 6000 {
		t.Fatalf("bounds: %d/%d/%d", len(b.Entries), b.Dropped, b.Last)
	}
	for i := 1; i < len(b.Entries); i++ {
		if b.Entries[i].Seq != b.Entries[i-1].Seq+1 {
			t.Fatal("out of order")
		}
	}
	for i := 0; i < 200; i++ {
		s.Append("gateway", 1, strings.Repeat("日志", EntryLimit))
	}
	if s.size > ByteLimit || len(s.Snapshot(0).Entries) >= Capacity {
		t.Fatal("byte budget was not enforced")
	}
	last := s.Snapshot(s.Version() - 1).Entries[0]
	if !strings.Contains(last.Raw, "已截断") {
		t.Fatal("missing truncation marker")
	}
}

func TestExportRequiresOverwriteAndPreservesFileOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.log")
	if err := Export(path, "first\n", false); err != nil {
		t.Fatal(err)
	}
	if err := Export(path, "second\n", false); !os.IsExist(err) {
		t.Fatalf("expected conflict: %v", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "first\n" {
		t.Fatal("existing file overwritten")
	}
	if err := Export(path, "second\n", true); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if string(b) != "second\n" {
		t.Fatal("confirmed export failed")
	}
	if err := Export(filepath.Join(path, "bad.log"), "bad", false); err == nil {
		t.Fatal("invalid destination accepted")
	}
}

func TestLongErrorRetainsFilterMetadata(t *testing.T) {
	s := New(nil)
	raw, _ := json.Marshal(map[string]string{"level": "ERROR", "msg": strings.Repeat("设备超时", EntryLimit), "gateway": "line-a", "err": strings.Repeat("error", EntryLimit)})
	s.Append("gateway", 1, string(raw))
	e := s.Snapshot(0).Entries[0]
	if e.Level != "ERROR" || e.Gateway != "line-a" || !strings.Contains(e.Message, "已截断") || !strings.Contains(e.Fields, "已截断") {
		t.Fatalf("metadata or limit lost: %s / %s", e.Level, e.Gateway)
	}
	if s.size > ByteLimit {
		t.Fatal("large fields exceeded budget")
	}
}
