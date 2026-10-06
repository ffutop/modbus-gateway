package cli

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type failedLogWriter struct{}

func (failedLogWriter) Write(p []byte) (int, error) { return 0, errors.New("disk full") }

func TestMirroredLoggerKeepsStructuredStreamWhenFileFails(t *testing.T) {
	var stream bytes.Buffer
	h := mirroredHandler{slog.NewTextHandler(failedLogWriter{}, nil), slog.NewJSONHandler(&stream, nil)}
	l := slog.New(h).With("gateway", "line-a").WithGroup("connection")
	l.Error("failed", "err", "timeout")
	text := stream.String()
	for _, want := range []string{`"level":"ERROR"`, `"gateway":"line-a"`, `"connection":{"err":"timeout"}`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s: %s", want, text)
		}
	}
	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("debug unexpectedly enabled")
	}
}
