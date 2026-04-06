package logger

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func zeroTime() time.Time { return time.Time{} }

// newRecord is a helper to build a slog.Record without a pc (0 = no source).
func newRecord(level slog.Level, msg string) slog.Record {
	return slog.NewRecord(zeroTime(), level, msg, 0)
}

// ---------------------------------------------------------------------------
// ConsoleHandler
// ---------------------------------------------------------------------------

func TestConsoleHandler_Enabled(t *testing.T) {
	h := NewConsoleHandler(&bytes.Buffer{}, slog.LevelInfo)

	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected INFO to be enabled at INFO level")
	}
	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected DEBUG to be disabled at INFO level")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("expected WARN to be enabled at INFO level")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("expected ERROR to be enabled at INFO level")
	}
}

func TestConsoleHandler_Handle_WritesMessage(t *testing.T) {
	var buf bytes.Buffer
	h := NewConsoleHandler(&buf, slog.LevelDebug)

	r := newRecord(slog.LevelInfo, "hello world")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected 'hello world' in output, got: %q", out)
	}
}

func TestConsoleHandler_Handle_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewConsoleHandler(&buf, slog.LevelDebug)
	h2 := h.WithAttrs([]slog.Attr{slog.String("key", "value")})

	r := newRecord(slog.LevelInfo, "msg")
	if err := h2.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "key") || !strings.Contains(out, "value") {
		t.Errorf("expected key=value in output, got: %q", out)
	}
}

func TestConsoleHandler_Handle_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := NewConsoleHandler(&buf, slog.LevelDebug)
	h2 := h.WithGroup("grp")

	r := newRecord(slog.LevelInfo, "msg")
	r.AddAttrs(slog.String("field", "val"))
	if err := h2.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle with group: %v", err)
	}
}

func TestConsoleHandler_AllLevels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError,
	}
	for _, lvl := range levels {
		t.Run(lvl.String(), func(t *testing.T) {
			var buf bytes.Buffer
			h := NewConsoleHandler(&buf, slog.LevelDebug)
			r := newRecord(lvl, "test message")
			if err := h.Handle(context.Background(), r); err != nil {
				t.Fatalf("Handle at level %v: %v", lvl, err)
			}
			if !strings.Contains(buf.String(), "test message") {
				t.Errorf("expected message in output at level %v", lvl)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// teeHandler
// ---------------------------------------------------------------------------

func TestTeeHandler_FansOut(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	h1 := NewConsoleHandler(&buf1, slog.LevelDebug)
	h2 := NewConsoleHandler(&buf2, slog.LevelDebug)

	tee := &teeHandler{handlers: []slog.Handler{h1, h2}}

	r := newRecord(slog.LevelInfo, "broadcast")
	if err := tee.Handle(context.Background(), r); err != nil {
		t.Fatalf("teeHandler.Handle: %v", err)
	}

	if !strings.Contains(buf1.String(), "broadcast") {
		t.Error("handler 1 should have received the record")
	}
	if !strings.Contains(buf2.String(), "broadcast") {
		t.Error("handler 2 should have received the record")
	}
}

func TestTeeHandler_Enabled(t *testing.T) {
	h1 := NewConsoleHandler(&bytes.Buffer{}, slog.LevelError)
	h2 := NewConsoleHandler(&bytes.Buffer{}, slog.LevelDebug)
	tee := &teeHandler{handlers: []slog.Handler{h1, h2}}

	if !tee.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("tee should be enabled when any handler is enabled")
	}
}

func TestTeeHandler_WithAttrs(t *testing.T) {
	h := NewConsoleHandler(&bytes.Buffer{}, slog.LevelDebug)
	tee := &teeHandler{handlers: []slog.Handler{h}}
	newTee := tee.WithAttrs([]slog.Attr{slog.String("k", "v")})
	if newTee == nil {
		t.Error("WithAttrs should not return nil")
	}
}

func TestTeeHandler_WithGroup(t *testing.T) {
	h := NewConsoleHandler(&bytes.Buffer{}, slog.LevelDebug)
	tee := &teeHandler{handlers: []slog.Handler{h}}
	newTee := tee.WithGroup("grp")
	if newTee == nil {
		t.Error("WithGroup should not return nil")
	}
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

func TestSetup_NoLogFile(t *testing.T) {
	if err := Setup(slog.LevelInfo, ""); err != nil {
		t.Fatalf("Setup without log file: %v", err)
	}
}

func TestSetup_WithLogFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	if err := Setup(slog.LevelDebug, logFile); err != nil {
		t.Fatalf("Setup with log file: %v", err)
	}
	// Emit a record so the file is written.
	slog.Info("setup test")

	if _, err := os.Stat(logFile); err != nil {
		t.Error("expected log file to be created")
	}
}

func TestSetup_InvalidLogFile(t *testing.T) {
	err := Setup(slog.LevelInfo, "/no/such/directory/test.log")
	if err == nil {
		t.Fatal("expected error for invalid log file path")
	}
}

// ---------------------------------------------------------------------------
// Spinner (non-TTY — safe to run in tests)
// ---------------------------------------------------------------------------

func TestStartSpinner_NonTTY_ReturnsNil(t *testing.T) {
	var buf bytes.Buffer
	s := StartSpinner(&buf, "working")
	if s != nil {
		t.Error("expected nil Spinner for non-TTY writer")
	}
}

func TestSpinner_NilSafe(t *testing.T) {
	var s *Spinner
	// All methods must be no-ops on nil.
	s.Update("new label")
	s.Done(true, "done")
}

// ---------------------------------------------------------------------------
// writeAttr and fmtValue with various value types
// ---------------------------------------------------------------------------

func TestConsoleHandler_Handle_NumericAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewConsoleHandler(&buf, slog.LevelDebug)
	r := newRecord(slog.LevelInfo, "nums")
	r.AddAttrs(
		slog.Int("i", 42),
		slog.Uint64("u", 99),
		slog.Float64("f", 3.14),
		slog.Bool("b", true),
		slog.Duration("d", 1000000000), // 1s
	)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle with numeric attrs: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "42") {
		t.Errorf("expected integer value in output: %q", out)
	}
}

func TestConsoleHandler_Handle_GroupAttr(t *testing.T) {
	var buf bytes.Buffer
	h := NewConsoleHandler(&buf, slog.LevelDebug)
	r := newRecord(slog.LevelInfo, "group test")
	r.AddAttrs(slog.Group("grp", slog.String("k", "v")))
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle with group attr: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "v") {
		t.Errorf("expected group value in output: %q", out)
	}
}

func TestConsoleHandler_Handle_SpacedString(t *testing.T) {
	// Strings with spaces should be quoted.
	var buf bytes.Buffer
	h := NewConsoleHandler(&buf, slog.LevelDebug)
	r := newRecord(slog.LevelInfo, "spaced")
	r.AddAttrs(slog.String("msg", "hello world"))
	h.Handle(context.Background(), r)
	out := buf.String()
	if !strings.Contains(out, `"hello world"`) {
		t.Errorf("expected quoted string in output: %q", out)
	}
}

func TestConsoleHandler_WithGroup_NestedPrefix(t *testing.T) {
	var buf bytes.Buffer
	h := NewConsoleHandler(&buf, slog.LevelDebug)
	h2 := h.WithGroup("outer")
	h3 := h2.WithGroup("inner")
	r := newRecord(slog.LevelInfo, "nested group")
	r.AddAttrs(slog.String("k", "v"))
	if err := h3.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle nested groups: %v", err)
	}
}

func TestTeeHandler_Enabled_AllDisabled(t *testing.T) {
	// Both handlers are ERROR-level, asking about DEBUG should return false.
	h1 := NewConsoleHandler(&bytes.Buffer{}, slog.LevelError)
	h2 := NewConsoleHandler(&bytes.Buffer{}, slog.LevelError)
	tee := &teeHandler{handlers: []slog.Handler{h1, h2}}

	if tee.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected false when all handlers have level >= DEBUG")
	}
}

func TestConsoleHandler_Handle_TimeAttr(t *testing.T) {
	var buf bytes.Buffer
	h := NewConsoleHandler(&buf, slog.LevelDebug)
	r := newRecord(slog.LevelInfo, "with time")
	r.AddAttrs(slog.Time("ts", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)))
	h.Handle(context.Background(), r)
	out := buf.String()
	if !strings.Contains(out, "2024") {
		t.Errorf("expected year in time output: %q", out)
	}
}

func TestConsoleHandler_Handle_AnyAttr(t *testing.T) {
	var buf bytes.Buffer
	h := NewConsoleHandler(&buf, slog.LevelDebug)
	r := newRecord(slog.LevelInfo, "any attr")
	r.AddAttrs(slog.Any("custom", struct{ X int }{X: 42}))
	h.Handle(context.Background(), r)
	// Should not panic and should produce output.
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

// ---------------------------------------------------------------------------
// isTTY — non-file writer returns false
// ---------------------------------------------------------------------------

func TestIsTTY_NonFileWriter(t *testing.T) {
	var buf bytes.Buffer
	if isTTY(&buf) {
		t.Error("expected isTTY=false for bytes.Buffer")
	}
}

func TestIsTTY_RegularFile(t *testing.T) {
	// *os.File that is not a character device — isTTY must return false.
	f, err := os.CreateTemp("", "gaal-tty-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())
	if isTTY(f) {
		t.Error("expected isTTY=false for a regular temp file")
	}
}

// ---------------------------------------------------------------------------
// Spinner — non-nil paths (run goroutine, Update, Done, Stop, clearLocked)
// ---------------------------------------------------------------------------

func makeSpinner(out *bytes.Buffer) *Spinner {
	return &Spinner{
		out:     out,
		label:   "testing",
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func TestSpinner_Update_NonNil(t *testing.T) {
	var buf bytes.Buffer
	s := makeSpinner(&buf)
	go s.run()
	s.Update("updated label")
	s.Stop()
}

func TestSpinner_Stop_NonNil(t *testing.T) {
	var buf bytes.Buffer
	s := makeSpinner(&buf)
	go s.run()
	// Wait for at least one tick so the ticker branch in run() is covered.
	time.Sleep(100 * time.Millisecond)
	s.Stop()
}

func TestSpinner_Done_OK_NonNil(t *testing.T) {
	var buf bytes.Buffer
	s := makeSpinner(&buf)
	go s.run()
	s.Done(true, "success")
	if !strings.Contains(buf.String(), "✓") {
		t.Errorf("expected checkmark in Done(true) output, got: %q", buf.String())
	}
}

func TestSpinner_Done_Fail_NonNil(t *testing.T) {
	var buf bytes.Buffer
	s := makeSpinner(&buf)
	go s.run()
	s.Done(false, "failure")
	if !strings.Contains(buf.String(), "✗") {
		t.Errorf("expected cross in Done(false) output, got: %q", buf.String())
	}
}

func TestSpinner_ClearLocked_Drawn(t *testing.T) {
	var buf bytes.Buffer
	s := makeSpinner(&buf)
	// Set drawn=true so clearLocked() actually writes escape codes.
	ttyMu.Lock()
	s.drawn = true
	s.clearLocked()
	ttyMu.Unlock()
	if s.drawn {
		t.Error("expected drawn=false after clearLocked")
	}
}

// ---------------------------------------------------------------------------
// ConsoleHandler.Handle with color=true (TTY path, no real TTY needed)
// ---------------------------------------------------------------------------

func TestConsoleHandler_Handle_ColorPath(t *testing.T) {
	var buf bytes.Buffer
	// Manually construct a handler with color=true bypassing the isTTY check.
	h := &ConsoleHandler{
		out:   &buf,
		level: slog.LevelDebug,
		color: true,
	}
	r := newRecord(slog.LevelInfo, "color message")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle with color: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected output from color Handle")
	}
}

func TestConsoleHandler_Handle_ColorPath_WithActiveSpinner(t *testing.T) {
	var buf bytes.Buffer
	h := &ConsoleHandler{
		out:   &buf,
		level: slog.LevelDebug,
		color: true,
	}
	// Set a fake active spinner so clearLocked is called inside Handle.
	spinnerBuf := bytes.Buffer{}
	fake := makeSpinner(&spinnerBuf)
	fake.drawn = true
	ttyMu.Lock()
	activeSpinner = fake
	ttyMu.Unlock()
	t.Cleanup(func() {
		ttyMu.Lock()
		activeSpinner = nil
		ttyMu.Unlock()
	})

	r := newRecord(slog.LevelWarn, "warning with spinner")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle with spinner active: %v", err)
	}
}
