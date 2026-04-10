package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
)

// ANSI SGR codes.
const (
	ansiReset    = "\033[0m"
	ansiGreen    = "\033[32m"   // INFO message
	ansiYellow   = "\033[33m"   // WARN message
	ansiBlue     = "\033[34m"   // DEBUG message
	ansiRed      = "\033[31m"   // ERROR message
	ansiBold     = "\033[1m"    // ERROR (bold prefix)
	ansiDimCyan  = "\033[2;36m" // attribute key
	ansiDimWhite = "\033[2;37m" // attribute value
)

func colorFor(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return ansiBlue
	case l < slog.LevelWarn:
		return ansiGreen
	case l < slog.LevelError:
		return ansiYellow
	default:
		return ansiBold + ansiRed
	}
}

// ConsoleHandler is a slog.Handler that produces compact, colorized log lines.
//
// The entire line (message + attributes) is colored according to the log level.
// No timestamp, no level tag — just signal through color:
//
//	message text  key=value  key2=value2
type ConsoleHandler struct {
	mu     sync.Mutex
	out    io.Writer
	color  bool
	level  slog.Level
	attrs  []slog.Attr // pre-attached via WithAttrs
	prefix string      // group prefix via WithGroup
}

// NewConsoleHandler returns a ConsoleHandler writing to out.
// Color is auto-detected: enabled when out is an *os.File attached to a TTY.
func NewConsoleHandler(out io.Writer, level slog.Level) *ConsoleHandler {
	color := false
	if f, ok := out.(*os.File); ok {
		if fi, err := f.Stat(); err == nil {
			color = fi.Mode()&os.ModeCharDevice != 0
		}
	}
	return &ConsoleHandler{out: out, color: color, level: level}
}

func (h *ConsoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *ConsoleHandler) Handle(_ context.Context, r slog.Record) error {
	var buf bytes.Buffer

	color := colorFor(r.Level)

	// Open color scope for the whole line.
	if h.color {
		buf.WriteString(color)
	}

	// ── Message ──────────────────────────────────────────────────────────
	buf.WriteString(r.Message)

	// ── Attrs: pre-attached + record ─────────────────────────────────────
	for _, a := range h.attrs {
		writeAttr(&buf, h.prefix, a, h.color)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&buf, h.prefix, a, h.color)
		return true
	})

	// Close color scope.
	if h.color {
		buf.WriteString(ansiReset)
	}

	buf.WriteByte('\n')

	if h.color {
		// TTY path: coordinate with the spinner via the shared ttyMu so that
		// log lines and the spinner animation never interleave.
		ttyMu.Lock()
		defer ttyMu.Unlock()
		if activeSpinner != nil {
			activeSpinner.clearLocked() // erase spinner line before our line
		}
		_, err := h.out.Write(buf.Bytes())
		return err
	}

	// Non-TTY path: simple per-handler mutex.
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(buf.Bytes())
	return err
}

func (h *ConsoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &ConsoleHandler{
		out:    h.out,
		color:  h.color,
		level:  h.level,
		attrs:  merged,
		prefix: h.prefix,
	}
}

func (h *ConsoleHandler) WithGroup(name string) slog.Handler {
	prefix := name
	if h.prefix != "" {
		prefix = h.prefix + "." + name
	}
	return &ConsoleHandler{
		out:    h.out,
		color:  h.color,
		level:  h.level,
		attrs:  h.attrs,
		prefix: prefix,
	}
}

// writeAttr appends a single key=value pair to buf.
// Key is in dim cyan, value in dim white when color is enabled.
func writeAttr(buf *bytes.Buffer, prefix string, a slog.Attr, color bool) {
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}

	// Recurse into groups.
	if a.Value.Kind() == slog.KindGroup {
		for _, sub := range a.Value.Group() {
			writeAttr(buf, key, sub, color)
		}
		return
	}

	buf.WriteString("  ")
	if color {
		buf.WriteString(ansiDimCyan)
	}
	buf.WriteString(key)
	if color {
		buf.WriteString(ansiReset)
	}
	buf.WriteByte('=')
	if color {
		buf.WriteString(ansiDimWhite)
	}
	buf.WriteString(fmtValue(a.Value))
	if color {
		buf.WriteString(ansiReset)
	}
}

// fmtValue converts a slog.Value to its display string.
func fmtValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if strings.ContainsAny(s, " \t\n") {
			return strconv.Quote(s)
		}
		return s
	case slog.KindInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'f', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format("2006-01-02T15:04:05Z07:00")
	default:
		return fmt.Sprintf("%v", v.Any())
	}
}
