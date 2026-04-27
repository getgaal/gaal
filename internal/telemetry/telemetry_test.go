package telemetry

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// resetGlobals resets package-level state between tests.
func resetGlobals() {
	enabled = false
	httpClient = nil
	baseProps = nil
	statePath = ""
	appVersion = ""
	pendingConsentWrite = nil
}

func TestTrackSendsPageviewWhenEnabled(t *testing.T) {
	resetGlobals()
	defer resetGlobals()

	var called atomic.Bool
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		called.Store(true)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	enabled = true
	httpClient = &client{endpoint: srv.URL, userAgent: "gaal/test"}
	baseProps = map[string]string{"version": "1.0.0"}

	Track("install")

	// Give the goroutine time to fire.
	deadline := time.Now().Add(2 * time.Second)
	for !called.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
	}

	if !called.Load() {
		t.Fatal("expected HTTP call, but server was not contacted")
	}

	var p plausiblePayload
	if err := json.Unmarshal(capturedBody, &p); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if p.Name != "pageview" {
		t.Errorf("Name = %q, want %q", p.Name, "pageview")
	}
	if p.URL != "app://gaal/cmd/install" {
		t.Errorf("URL = %q, want %q", p.URL, "app://gaal/cmd/install")
	}
	if p.Domain != plausibleDomain {
		t.Errorf("Domain = %q, want %q", p.Domain, plausibleDomain)
	}
	if p.Props["version"] != "1.0.0" {
		t.Errorf("Props[version] = %q, want %q", p.Props["version"], "1.0.0")
	}
}

func TestTrackNoopWhenDisabled(t *testing.T) {
	resetGlobals()
	defer resetGlobals()

	var called atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	enabled = false
	httpClient = &client{endpoint: srv.URL, userAgent: "gaal/test"}
	baseProps = map[string]string{"version": "1.0.0"}

	Track("install")

	// Give some time to verify no call is made.
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}

	if called.Load() {
		t.Error("expected no HTTP call when disabled, but server was contacted")
	}
}

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"yaml parse error with parsing yaml", errors.New("error parsing yaml file"), "yaml_parse_error"},
		{"yaml parse error with invalid config", errors.New("invalid config format"), "yaml_parse_error"},
		{"agent not found", errors.New("agent 'foo' not found in registry"), "agent_not_found"},
		{"sync failed", errors.New("sync failed: timeout"), "sync_failed"},
		{"permission denied", errors.New("open /etc/config: permission denied"), "permission_denied"},
		{"network dial error", errors.New("dial tcp 127.0.0.1:443: connection refused"), "network_error"},
		{"network timeout", errors.New("request timeout after 5s"), "network_error"},
		{"network connection refused", errors.New("connection refused by server"), "network_error"},
		{"network generic", errors.New("network unreachable"), "network_error"},
		{"unknown error", errors.New("something unexpected happened"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := categorizeError(tt.err)
			if got != tt.expected {
				t.Errorf("categorizeError(%q) = %q, want %q", tt.err, got, tt.expected)
			}
		})
	}
}

func TestMilestoneState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".telemetry-state")

	// Load from non-existent file returns zero value.
	ms := loadMilestoneState(path)
	if ms.InstallSent || ms.FirstSyncSent {
		t.Error("expected zero-value milestoneState from non-existent file")
	}

	// Save and reload.
	ms.InstallSent = true
	saveMilestoneState(path, ms)

	ms2 := loadMilestoneState(path)
	if !ms2.InstallSent {
		t.Error("expected InstallSent=true after save/load")
	}
	if ms2.FirstSyncSent {
		t.Error("expected FirstSyncSent=false after save/load")
	}

	// Update and reload.
	ms2.FirstSyncSent = true
	saveMilestoneState(path, ms2)

	ms3 := loadMilestoneState(path)
	if !ms3.InstallSent || !ms3.FirstSyncSent {
		t.Error("expected both milestones true after second save/load")
	}
}

func TestMilestoneStateInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".telemetry-state")

	// Write invalid JSON.
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	ms := loadMilestoneState(path)
	if ms.InstallSent || ms.FirstSyncSent {
		t.Error("expected zero-value milestoneState from invalid JSON")
	}
}

func TestCopyProps(t *testing.T) {
	src := map[string]string{"a": "1", "b": "2"}
	dst := copyProps(src)

	// Should have same values.
	if dst["a"] != "1" || dst["b"] != "2" {
		t.Error("copy did not preserve values")
	}

	// Should be independent.
	dst["a"] = "changed"
	if src["a"] == "changed" {
		t.Error("copy is not independent of source")
	}
}
