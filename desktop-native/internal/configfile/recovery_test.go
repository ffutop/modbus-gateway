package configfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecoveryLatestWritePrivatePermissionsAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewRecovery(path, "original")
	for i := 0; i < 20; i++ {
		r.Queue("earlier")
	}
	r.Queue("latest")
	deadline := time.Now().Add(3 * time.Second)
	for {
		content, conflict, err := r.Load()
		if err == nil && content == "latest" {
			if conflict {
				t.Fatal("false conflict")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("recovery not saved: %s %v", content, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	info, err := os.Stat(r.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v %v", info, err)
	}
	r.Rebase("external change")
	text, conflict, err := r.Load()
	if r.Baseline() != "original" {
		t.Fatal("recovery lost historical baseline text")
	}
	if err != nil || text != "latest" || !conflict {
		t.Fatal("external change not detected")
	}
	if bytes, _ := os.ReadFile(path); string(bytes) != "original" {
		t.Fatal("recovery modified runnable config")
	}
}
func TestRecoveryClearInvalidatesQueuedWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	r := NewRecovery(path, "old")
	for i := 0; i < 100; i++ {
		r.Queue(strings.Repeat("draft", 100))
	}
	if err := r.Clear("saved"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := os.Stat(r.path); !os.IsNotExist(err) {
		t.Fatal("queued autosave resurrected discarded recovery")
	}
}
