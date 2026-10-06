package cli

import (
	"context"
	"errors"
	"log/slog"
)

// mirroredHandler always attempts both destinations: failure to write the
// configured file must not suppress the desktop diagnostic stream.
type mirroredHandler struct{ file, stream slog.Handler }

func (h mirroredHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.file.Enabled(ctx, level) || h.stream.Enabled(ctx, level)
}
func (h mirroredHandler) Handle(ctx context.Context, r slog.Record) error {
	var a, b error
	if h.file.Enabled(ctx, r.Level) {
		a = h.file.Handle(ctx, r.Clone())
	}
	if h.stream.Enabled(ctx, r.Level) {
		b = h.stream.Handle(ctx, r)
	}
	return errors.Join(a, b)
}
func (h mirroredHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return mirroredHandler{h.file.WithAttrs(attrs), h.stream.WithAttrs(attrs)}
}
func (h mirroredHandler) WithGroup(name string) slog.Handler {
	return mirroredHandler{h.file.WithGroup(name), h.stream.WithGroup(name)}
}
