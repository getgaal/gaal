package telemetry

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// consentState represents the resolved telemetry consent.
type consentState struct {
	Enabled     bool
	Source      string // "GAAL_TELEMETRY=0", "DO_NOT_TRACK=1", "config", "unconfigured"
	NeedsPrompt bool
}

// resolveState checks env vars and config to determine telemetry state.
// Precedence: GAAL_TELEMETRY > DO_NOT_TRACK > config value > unconfigured.
func resolveState(cfgValue *bool) consentState {
	if v, ok := os.LookupEnv("GAAL_TELEMETRY"); ok {
		switch strings.ToLower(v) {
		case "0", "false":
			return consentState{Enabled: false, Source: "GAAL_TELEMETRY=0"}
		case "1", "true":
			return consentState{Enabled: true, Source: "GAAL_TELEMETRY=1"}
		}
	}

	if v, ok := os.LookupEnv("DO_NOT_TRACK"); ok && v == "1" {
		return consentState{Enabled: false, Source: "DO_NOT_TRACK=1"}
	}

	if cfgValue != nil {
		return consentState{Enabled: *cfgValue, Source: "config"}
	}

	return consentState{Enabled: false, Source: "unconfigured", NeedsPrompt: true}
}

// persistConsent writes or updates the telemetry field in the user config file.
func persistConsent(cfgPath string, enabled bool) error {
	slog.Debug("persisting telemetry consent", "path", cfgPath, "enabled", enabled)

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	existing := make(map[string]any)
	data, err := os.ReadFile(cfgPath)
	if err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	existing["telemetry"] = enabled
	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(cfgPath, out, 0o644)
}
