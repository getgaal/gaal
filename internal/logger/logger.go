package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// Setup initialises the global slog logger.
//
// Console (stderr): compact, colorized output — level tags (INF/DBG/WRN/ERR),
// right-padded message, key=value attrs.
//
// File (logFile, optional): JSON lines including "host" and "time" fields,
// suitable for ingestion by log aggregators.
//
//	2026-04-05T16:10:30Z  INFO  sync started  config=gaal.yaml
func Setup(level slog.Level, logFile string) (func(), error) {
	consoleH := NewConsoleHandler(os.Stderr, level)

	var handler slog.Handler = consoleH
	noop := func() {}

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) //nolint:gosec
		if err != nil {
			return noop, fmt.Errorf("opening log file %q: %w", logFile, err)
		}

		hostname, _ := os.Hostname()
		jsonH := slog.NewJSONHandler(f, &slog.HandlerOptions{
			Level:     level,
			AddSource: false,
		}).WithAttrs([]slog.Attr{slog.String("host", hostname)})

		handler = &teeHandler{handlers: []slog.Handler{consoleH, jsonH}}
		slog.SetDefault(slog.New(handler))
		// Return a teardown that closes the file and resets slog.
		// Required on Windows where open file handles prevent TempDir cleanup.
		return func() {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
			_ = f.Close()
		}, nil
	}

	slog.SetDefault(slog.New(handler))
	return noop, nil
}

// teeHandler fans out records to multiple handlers.
type teeHandler struct {
	handlers []slog.Handler
}

func (t *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range t.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (t *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range t.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		hs[i] = h.WithAttrs(attrs)
	}
	return &teeHandler{handlers: hs}
}

func (t *teeHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		hs[i] = h.WithGroup(name)
	}
	return &teeHandler{handlers: hs}
}
