package logger

import (
	"context"
	"log/slog"

	"gaal/internal/urlx"
)

// redactingHandler wraps a slog.Handler and runs urlx.Redact on every string
// attribute value (including those nested inside groups) before forwarding the
// record to the inner handler.
//
// This is a defence-in-depth safeguard: even if a log site forgets to call
// urlx.Redact or urlx.SlogURL, credentials embedded in URL strings cannot
// reach the underlying handler — and therefore cannot leak into --verbose
// output, log files, or any other persisted surface.
type redactingHandler struct {
	inner slog.Handler
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	clone := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		clone.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, clone)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = redactAttr(a)
	}
	return &redactingHandler{inner: h.inner.WithAttrs(redacted)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name)}
}

// redactAttr returns a copy of a with any string value redacted via urlx.Redact.
// Group attrs are recursed into; non-string, non-group values are returned as-is.
func redactAttr(a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, urlx.Redact(a.Value.String()))
	case slog.KindGroup:
		group := a.Value.Group()
		out := make([]slog.Attr, len(group))
		for i, sub := range group {
			out[i] = redactAttr(sub)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	default:
		return a
	}
}
