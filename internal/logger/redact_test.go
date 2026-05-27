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

// captureHandler is a tiny slog.Handler that records the most recent Record
// (and any pre-attached attrs / group prefix) for assertions.
type captureHandler struct {
	enabledLevel slog.Level
	rec          slog.Record
	attrs        []slog.Attr
	group        string
	handled      bool
	enabledCalls int
}

func (c *captureHandler) Enabled(_ context.Context, level slog.Level) bool {
	c.enabledCalls++
	return level >= c.enabledLevel
}

func (c *captureHandler) Handle(_ context.Context, r slog.Record) error {
	c.rec = r
	c.handled = true
	return nil
}

func (c *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(c.attrs)+len(attrs))
	merged = append(merged, c.attrs...)
	merged = append(merged, attrs...)
	return &captureHandler{enabledLevel: c.enabledLevel, attrs: merged, group: c.group}
}

func (c *captureHandler) WithGroup(name string) slog.Handler {
	g := name
	if c.group != "" {
		g = c.group + "." + name
	}
	return &captureHandler{enabledLevel: c.enabledLevel, attrs: c.attrs, group: g}
}

func collectAttrs(r slog.Record) map[string]slog.Value {
	out := map[string]slog.Value{}
	r.Attrs(func(a slog.Attr) bool {
		out[a.Key] = a.Value
		return true
	})
	return out
}

func TestRedactAttr(t *testing.T) {
	tests := []struct {
		name string
		in   slog.Attr
		want slog.Attr
	}{
		{
			name: "credential url stripped",
			in:   slog.String("url", "https://alice:secret@example.com/repo"),
			want: slog.String("url", "https://example.com/repo"),
		},
		{
			name: "plain string unchanged",
			in:   slog.String("msg", "no credentials here"),
			want: slog.String("msg", "no credentials here"),
		},
		{
			name: "plain url unchanged",
			in:   slog.String("url", "https://github.com/owner/repo.git"),
			want: slog.String("url", "https://github.com/owner/repo.git"),
		},
		{
			name: "int unchanged",
			in:   slog.Int("count", 42),
			want: slog.Int("count", 42),
		},
		{
			name: "bool unchanged",
			in:   slog.Bool("ok", true),
			want: slog.Bool("ok", true),
		},
		{
			name: "duration unchanged",
			in:   slog.Duration("d", time.Second),
			want: slog.Duration("d", time.Second),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactAttr(tt.in)
			if got.Key != tt.want.Key {
				t.Errorf("key = %q, want %q", got.Key, tt.want.Key)
			}
			if got.Value.String() != tt.want.Value.String() {
				t.Errorf("value = %q, want %q", got.Value.String(), tt.want.Value.String())
			}
		})
	}
}

func TestRedactAttr_NestedGroup(t *testing.T) {
	in := slog.Group("outer",
		slog.String("url", "https://u:p@host/x"),
		slog.Group("inner",
			slog.String("nested", "https://a:b@host2/y"),
			slog.Int("count", 7),
		),
	)
	got := redactAttr(in)

	if got.Value.Kind() != slog.KindGroup {
		t.Fatalf("outer kind = %v, want Group", got.Value.Kind())
	}
	outer := got.Value.Group()
	if len(outer) != 2 {
		t.Fatalf("outer len = %d, want 2", len(outer))
	}

	if outer[0].Value.String() != "https://host/x" {
		t.Errorf("outer.url = %q, want %q", outer[0].Value.String(), "https://host/x")
	}

	if outer[1].Value.Kind() != slog.KindGroup {
		t.Fatalf("inner kind = %v, want Group", outer[1].Value.Kind())
	}
	inner := outer[1].Value.Group()
	if inner[0].Value.String() != "https://host2/y" {
		t.Errorf("inner.nested = %q, want %q", inner[0].Value.String(), "https://host2/y")
	}
	if inner[1].Value.Int64() != 7 {
		t.Errorf("inner.count = %d, want 7", inner[1].Value.Int64())
	}
}

func TestRedactingHandler_Handle_RedactsString(t *testing.T) {
	cap := &captureHandler{enabledLevel: slog.LevelDebug}
	h := &redactingHandler{inner: cap}

	r := newRecord(slog.LevelInfo, "syncing")
	r.AddAttrs(slog.String("url", "https://alice:secret@example.com/repo"))

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !cap.handled {
		t.Fatal("inner handler was not called")
	}
	attrs := collectAttrs(cap.rec)
	if got := attrs["url"].String(); got != "https://example.com/repo" {
		t.Errorf("url attr = %q, want %q", got, "https://example.com/repo")
	}
	// Original record must not have been mutated.
	origAttrs := collectAttrs(r)
	if got := origAttrs["url"].String(); got != "https://alice:secret@example.com/repo" {
		t.Errorf("original record mutated: url = %q", got)
	}
}

func TestRedactingHandler_Handle_PreservesNonString(t *testing.T) {
	cap := &captureHandler{enabledLevel: slog.LevelDebug}
	h := &redactingHandler{inner: cap}

	r := newRecord(slog.LevelInfo, "msg")
	r.AddAttrs(
		slog.Int("count", 3),
		slog.Bool("ok", true),
	)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	attrs := collectAttrs(cap.rec)
	if attrs["count"].Int64() != 3 {
		t.Errorf("count = %d, want 3", attrs["count"].Int64())
	}
	if !attrs["ok"].Bool() {
		t.Errorf("ok = false, want true")
	}
}

func TestRedactingHandler_Handle_PreservesRecordMetadata(t *testing.T) {
	cap := &captureHandler{enabledLevel: slog.LevelDebug}
	h := &redactingHandler{inner: cap}

	ts := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	r := slog.NewRecord(ts, slog.LevelWarn, "hello", 0)
	r.AddAttrs(slog.String("k", "v"))

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if cap.rec.Message != "hello" {
		t.Errorf("message = %q, want %q", cap.rec.Message, "hello")
	}
	if cap.rec.Level != slog.LevelWarn {
		t.Errorf("level = %v, want WARN", cap.rec.Level)
	}
	if !cap.rec.Time.Equal(ts) {
		t.Errorf("time = %v, want %v", cap.rec.Time, ts)
	}
}

func TestRedactingHandler_Enabled_Forwards(t *testing.T) {
	cap := &captureHandler{enabledLevel: slog.LevelWarn}
	h := &redactingHandler{inner: cap}

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("DEBUG should be disabled when inner is WARN")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("ERROR should be enabled when inner is WARN")
	}
	if cap.enabledCalls != 2 {
		t.Errorf("inner Enabled called %d times, want 2", cap.enabledCalls)
	}
}

func TestRedactingHandler_WithAttrs_RedactsAndDelegates(t *testing.T) {
	cap := &captureHandler{enabledLevel: slog.LevelDebug}
	h := &redactingHandler{inner: cap}

	derived := h.WithAttrs([]slog.Attr{
		slog.String("url", "https://u:p@host/x"),
		slog.Int("n", 1),
	})

	rh, ok := derived.(*redactingHandler)
	if !ok {
		t.Fatalf("WithAttrs returned %T, want *redactingHandler", derived)
	}
	innerCap, ok := rh.inner.(*captureHandler)
	if !ok {
		t.Fatalf("inner = %T, want *captureHandler", rh.inner)
	}
	if len(innerCap.attrs) != 2 {
		t.Fatalf("inner attrs len = %d, want 2", len(innerCap.attrs))
	}
	if got := innerCap.attrs[0].Value.String(); got != "https://host/x" {
		t.Errorf("attached url = %q, want %q", got, "https://host/x")
	}
	if innerCap.attrs[1].Value.Int64() != 1 {
		t.Errorf("attached n = %d, want 1", innerCap.attrs[1].Value.Int64())
	}
}

func TestRedactingHandler_WithGroup_Delegates(t *testing.T) {
	cap := &captureHandler{enabledLevel: slog.LevelDebug}
	h := &redactingHandler{inner: cap}

	derived := h.WithGroup("grp")
	rh, ok := derived.(*redactingHandler)
	if !ok {
		t.Fatalf("WithGroup returned %T, want *redactingHandler", derived)
	}
	innerCap, ok := rh.inner.(*captureHandler)
	if !ok {
		t.Fatalf("inner = %T, want *captureHandler", rh.inner)
	}
	if innerCap.group != "grp" {
		t.Errorf("inner group = %q, want %q", innerCap.group, "grp")
	}
}

func TestSetup_RedactsCredentialsInLogFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "redact-e2e.log")
	teardown, err := Setup(slog.LevelDebug, logFile)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(teardown)

	// Log without calling Redact/SlogURL explicitly — the wrapper must scrub.
	slog.Info("cloning", "url", "https://alice:hunter2@github.com/owner/repo.git")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	out := string(data)
	if strings.Contains(out, "alice") || strings.Contains(out, "hunter2") {
		t.Errorf("credentials leaked into log file: %q", out)
	}
	if !strings.Contains(out, "github.com/owner/repo.git") {
		t.Errorf("expected sanitised url in log file: %q", out)
	}
}

func TestSetup_NoLogFile_RedactsOnConsole(t *testing.T) {
	// Redirect stderr so we can inspect what the console handler writes.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
		slog.SetDefault(slog.New(slog.NewTextHandler(origStderr, nil)))
	})

	if _, err := Setup(slog.LevelDebug, ""); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	slog.Info("cloning", "url", "https://bob:topsecret@example.com/x")

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	out := buf.String()
	if strings.Contains(out, "bob") || strings.Contains(out, "topsecret") {
		t.Errorf("credentials leaked into stderr: %q", out)
	}
	if !strings.Contains(out, "example.com/x") {
		t.Errorf("expected sanitised url in stderr: %q", out)
	}
}
