package telemetry

import (
	"os"
	"path/filepath"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestResolveStateEnvGaalTelemetryDisabled(t *testing.T) {
	t.Setenv("GAAL_TELEMETRY", "0")
	s := resolveState(nil)
	if s.Enabled {
		t.Error("expected Enabled=false")
	}
	if s.Source != "GAAL_TELEMETRY=0" {
		t.Errorf("expected source GAAL_TELEMETRY=0, got %q", s.Source)
	}
	if s.NeedsPrompt {
		t.Error("expected NeedsPrompt=false")
	}
}

func TestResolveStateEnvGaalTelemetryEnabled(t *testing.T) {
	t.Setenv("GAAL_TELEMETRY", "1")
	s := resolveState(nil)
	if !s.Enabled {
		t.Error("expected Enabled=true")
	}
	if s.Source != "GAAL_TELEMETRY=1" {
		t.Errorf("expected source GAAL_TELEMETRY=1, got %q", s.Source)
	}
}

func TestResolveStateEnvDoNotTrack(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	s := resolveState(nil)
	if s.Enabled {
		t.Error("expected Enabled=false")
	}
	if s.Source != "DO_NOT_TRACK=1" {
		t.Errorf("expected source DO_NOT_TRACK=1, got %q", s.Source)
	}
}

func TestResolveStateConfigTrue(t *testing.T) {
	s := resolveState(boolPtr(true))
	if !s.Enabled {
		t.Error("expected Enabled=true")
	}
	if s.Source != "config" {
		t.Errorf("expected source config, got %q", s.Source)
	}
}

func TestResolveStateConfigFalse(t *testing.T) {
	s := resolveState(boolPtr(false))
	if s.Enabled {
		t.Error("expected Enabled=false")
	}
	if s.Source != "config" {
		t.Errorf("expected source config, got %q", s.Source)
	}
}

func TestResolveStateUnconfigured(t *testing.T) {
	s := resolveState(nil)
	if s.Enabled {
		t.Error("expected Enabled=false")
	}
	if s.Source != "unconfigured" {
		t.Errorf("expected source unconfigured, got %q", s.Source)
	}
	if !s.NeedsPrompt {
		t.Error("expected NeedsPrompt=true")
	}
}

func TestEnvOverridesConfig(t *testing.T) {
	t.Setenv("GAAL_TELEMETRY", "0")
	s := resolveState(boolPtr(true))
	if s.Enabled {
		t.Error("expected Enabled=false: env should override config")
	}
	if s.Source != "GAAL_TELEMETRY=0" {
		t.Errorf("expected source GAAL_TELEMETRY=0, got %q", s.Source)
	}
}

func TestDoNotTrackOverridesConfig(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	s := resolveState(boolPtr(true))
	if s.Enabled {
		t.Error("expected Enabled=false: DO_NOT_TRACK should override config")
	}
	if s.Source != "DO_NOT_TRACK=1" {
		t.Errorf("expected source DO_NOT_TRACK=1, got %q", s.Source)
	}
}

func TestPersistConsent(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if err := persistConsent(cfgPath, true); err != nil {
		t.Fatalf("persistConsent failed: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	got := string(data)
	if got != "telemetry: true\n" {
		t.Errorf("expected %q, got %q", "telemetry: true\n", got)
	}
}

func TestPersistConsentPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	existing := []byte("some_key: some_value\n")
	if err := os.WriteFile(cfgPath, existing, 0o644); err != nil {
		t.Fatalf("writing existing config: %v", err)
	}

	if err := persistConsent(cfgPath, true); err != nil {
		t.Fatalf("persistConsent failed: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	got := string(data)
	if !contains(got, "telemetry: true") {
		t.Errorf("expected telemetry: true in output, got %q", got)
	}
	if !contains(got, "some_key: some_value") {
		t.Errorf("expected some_key: some_value preserved in output, got %q", got)
	}
}

func TestStatusEnabled(t *testing.T) {
	t.Setenv("GAAL_TELEMETRY", "1")
	status, source := Status(nil)
	if status != "enabled" {
		t.Fatalf("expected enabled, got %q", status)
	}
	if source != "GAAL_TELEMETRY=1" {
		t.Fatalf("expected source GAAL_TELEMETRY=1, got %q", source)
	}
}

func TestStatusDisabledEnv(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	status, source := Status(nil)
	if status != "disabled" {
		t.Fatalf("expected disabled, got %q", status)
	}
	if source != "DO_NOT_TRACK=1" {
		t.Fatalf("expected source DO_NOT_TRACK=1, got %q", source)
	}
}

func TestStatusUnconfigured(t *testing.T) {
	status, source := Status(nil)
	if status != "not configured" {
		t.Fatalf("expected not configured, got %q", status)
	}
	if source != "" {
		t.Fatalf("expected empty source, got %q", source)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
