// Package runlog retains diagnostic records independently of gateway lifetime
// and request telemetry. Only the desktop process owns this store.
package runlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	Capacity   = 5000
	ByteLimit  = 4 << 20
	EntryLimit = 16 << 10
)

type Entry struct {
	Seq, Session                                 uint64
	Time                                         time.Time
	Level, Source, Gateway, Message, Fields, Raw string
}

func (e Entry) Text() string {
	return fmt.Sprintf("%s %-7s %s session=%d %s", e.Time.Format(time.RFC3339Nano), e.Level, e.Source, e.Session, e.Raw)
}

type Snapshot struct {
	Entries       []Entry
	Last, Dropped uint64
}

type Source interface {
	Version() uint64
	Snapshot(after uint64) Snapshot
}

// Store has both a record and a byte bound. A zero Store is ready for use.
type Store struct {
	mu                    sync.Mutex
	ring                  [Capacity]Entry
	head, count, size     int
	seq, session, dropped uint64
	notify                func()
	lastNotify            time.Time
}

func New(notify func()) *Store { return &Store{notify: notify} }

func (s *Store) Version() uint64 { s.mu.Lock(); defer s.mu.Unlock(); return s.seq }

func (s *Store) BeginSession(path string) uint64 {
	s.mu.Lock()
	s.session++
	id := s.session
	s.mu.Unlock()
	s.Record("gateway", id, "INFO", "启动网关运行会话", "config", path)
	return id
}

func (s *Store) Record(source string, session uint64, level, message string, attrs ...string) {
	record := map[string]string{"time": time.Now().Format(time.RFC3339Nano), "level": level, "msg": message}
	for i := 0; i+1 < len(attrs); i += 2 {
		record[attrs[i]] = attrs[i+1]
	}
	b, _ := json.Marshal(record)
	s.Append(source, session, string(b))
}

func (s *Store) Append(source string, session uint64, raw string) {
	raw = strings.TrimSuffix(raw, "\r")
	if raw == "" {
		return
	}
	original := raw
	raw = LimitText(raw)
	e := Entry{Time: time.Now(), Level: "UNKNOWN", Source: source, Session: session, Message: raw, Raw: raw}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(original), &fields) == nil && fields != nil {
		var event string
		_ = json.Unmarshal(fields["event"], &event)
		if event == "ui_ready" {
			return
		} // protocol handshake, not a log record
		var timestamp string
		_ = json.Unmarshal(fields["time"], &timestamp)
		if t, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			e.Time = t
		}
		var level, message string
		_ = json.Unmarshal(fields["level"], &level)
		_ = json.Unmarshal(fields["msg"], &message)
		if message != "" {
			e.Message = message
		}
		switch strings.ToUpper(level) {
		case "DEBUG", "INFO", "WARN", "ERROR":
			e.Level = strings.ToUpper(level)
		}
		_ = json.Unmarshal(fields["gateway"], &e.Gateway)
		delete(fields, "time")
		delete(fields, "level")
		delete(fields, "msg")
		if len(fields) > 0 {
			b, _ := json.Marshal(fields)
			e.Fields = string(b)
		}
	}
	// Parse before truncating so long errors retain their level and gateway;
	// bound every stored string, including structured messages and attributes.
	e.Message, e.Fields, e.Gateway = LimitText(e.Message), LimitText(e.Fields), LimitText(e.Gateway)
	cost := entrySize(e)
	s.mu.Lock()
	s.seq++
	e.Seq = s.seq
	for s.count > 0 && (s.count == Capacity || s.size+cost > ByteLimit) {
		s.size -= entrySize(s.ring[s.head])
		s.ring[s.head] = Entry{}
		s.head = (s.head + 1) % Capacity
		s.count--
		s.dropped++
	}
	s.ring[(s.head+s.count)%Capacity] = e
	s.count++
	s.size += cost
	// A burst of logs must not schedule a render per record.
	wake := s.notify != nil && time.Since(s.lastNotify) >= 100*time.Millisecond
	if wake {
		s.lastNotify = time.Now()
	}
	s.mu.Unlock()
	if wake {
		s.notify()
	}
}

func LimitText(text string) string {
	text = strings.ToValidUTF8(text, "�")
	if len(text) <= EntryLimit {
		return text
	}
	n := EntryLimit
	for n > 0 && !utf8.ValidString(text[:n]) {
		n--
	}
	return text[:n] + " …[日志已截断]"
}

func entrySize(e Entry) int {
	return len(e.Raw) + len(e.Message) + len(e.Fields) + len(e.Gateway) + len(e.Source) + 128
}

func (s *Store) Snapshot(after uint64) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := Snapshot{Last: s.seq, Dropped: s.dropped}
	for i := 0; i < s.count; i++ {
		e := s.ring[(s.head+i)%Capacity]
		if e.Seq > after {
			result.Entries = append(result.Entries, e)
		}
	}
	return result
}

// Writer receives complete JSON lines from slog (one atomic Write per record).
type Writer struct {
	Store  *Store
	Source string
}

func (w Writer) Write(p []byte) (int, error) {
	for _, line := range bytes.Split(p, []byte{'\n'}) {
		w.Store.Append(w.Source, 0, string(line))
	}
	return len(p), nil
}
