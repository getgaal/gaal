package telemetry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gaal/internal/config"
	"gaal/internal/core/agent"
	"gaal/internal/skill"
)

// Package-level state, set once by Init.
var (
	enabled    bool
	httpClient *client
	baseProps  map[string]string
	statePath  string
	appVersion string
)

// milestoneState tracks which one-time events have already been sent.
type milestoneState struct {
	InstallSent   bool `json:"install_sent"`
	FirstSyncSent bool `json:"first_sync_sent"`
}

// Init resolves consent state and initialises the telemetry client.
// version is the gaal binary version string (passed from cmd.Version to avoid
// circular import).
func Init(cfgTelemetry *bool, promptFn func() (bool, error), version string) (bool, error) {
	appVersion = version

	state := resolveState(cfgTelemetry)
	slog.Debug("telemetry state resolved", "enabled", state.Enabled, "source", state.Source, "needsPrompt", state.NeedsPrompt)

	if state.NeedsPrompt && promptFn != nil {
		consent, err := promptFn()
		if err != nil {
			return false, fmt.Errorf("telemetry prompt: %w", err)
		}
		if err := persistConsent(config.UserConfigFilePath(), consent); err != nil {
			slog.Warn("failed to persist telemetry consent", "err", err)
		}
		state.Enabled = consent

		// Fire consent event regardless of the answer so we can track
		// opt-in vs opt-out rates (Plausible will only receive this if
		// consent == true since we check enabled below).
		if consent {
			// Temporarily enable so we can send the Consent event.
			enabled = true
			httpClient = newClient(version)
			baseProps = buildBaseProps()
			TrackCustom("Consent", map[string]string{"answer": "yes"})
		}
	}

	enabled = state.Enabled
	if !enabled {
		return false, nil
	}

	if httpClient == nil {
		httpClient = newClient(version)
	}
	if baseProps == nil {
		baseProps = buildBaseProps()
	}

	statePath = telemetryStatePath()

	// Fire Install milestone if not already sent.
	ms := loadMilestoneState(statePath)
	if !ms.InstallSent {
		TrackCustom("Install", nil)
		ms.InstallSent = true
		saveMilestoneState(statePath, ms)
	}

	return true, nil
}

// newClient creates an HTTP client with an appropriate user-agent string.
func newClient(version string) *client {
	ua := fmt.Sprintf("gaal/%s (%s; %s)", version, runtime.GOOS, runtime.GOARCH)
	return &client{
		endpoint:  plausibleEndpoint,
		userAgent: ua,
	}
}

// Track sends a pageview event for the given command in a fire-and-forget
// goroutine.
func Track(command string) {
	if !enabled {
		return
	}
	props := copyProps(baseProps)
	url := "app://gaal-lite/cmd/" + command
	slog.Debug("telemetry", "event", "pageview", "url", url, "props", props)
	go func() {
		p := plausiblePayload{
			Name:   "pageview",
			URL:    url,
			Domain: plausibleDomain,
			Props:  props,
		}
		if err := httpClient.send(p); err != nil {
			slog.Debug("telemetry pageview failed", "command", command, "err", err)
		}
	}()
}

// TrackCustom sends a named custom event with optional extra properties in a
// fire-and-forget goroutine.
func TrackCustom(name string, extra map[string]string) {
	if !enabled {
		return
	}
	props := copyProps(baseProps)
	for k, v := range extra {
		props[k] = v
	}
	url := "app://gaal-lite/custom/" + name
	slog.Debug("telemetry", "event", name, "url", url, "props", props)
	go func() {
		p := plausiblePayload{
			Name:   name,
			URL:    url,
			Domain: plausibleDomain,
			Props:  props,
		}
		if err := httpClient.send(p); err != nil {
			slog.Debug("telemetry custom event failed", "name", name, "err", err)
		}
	}()
}

// TrackError sends a categorised error event.
func TrackError(command string, err error) {
	category := categorizeError(err)
	TrackCustom("Error", map[string]string{
		"command":  command,
		"category": category,
		"message":  err.Error(),
	})
}

// TrackFirstSync sends the FirstSync milestone event once per installation.
func TrackFirstSync(agentCount int) {
	if !enabled {
		return
	}
	ms := loadMilestoneState(statePath)
	if ms.FirstSyncSent {
		return
	}
	TrackCustom("FirstSync", map[string]string{
		"agent_count": fmt.Sprintf("%d", agentCount),
	})
	ms.FirstSyncSent = true
	saveMilestoneState(statePath, ms)
}

// categorizeError returns a human-readable category for the error.
func categorizeError(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "parsing yaml") || strings.Contains(msg, "invalid config"):
		return "yaml_parse_error"
	case strings.Contains(msg, "agent") && strings.Contains(msg, "not found"):
		return "agent_not_found"
	case strings.Contains(msg, "sync"):
		return "sync_failed"
	case strings.Contains(msg, "permission"):
		return "permission_denied"
	case strings.Contains(msg, "dial") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "network"):
		return "network_error"
	default:
		return "unknown"
	}
}

// buildBaseProps returns the default properties attached to every event.
func buildBaseProps() map[string]string {
	agents := installedAgentNames()
	return map[string]string{
		"version": appVersion,
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"agents":  strings.Join(agents, ","),
	}
}

// installedAgentNames returns the names of agents currently installed on this
// machine. It uses agent.Names() for the full registry and
// skill.IsAgentInstalled() to check presence.
func installedAgentNames() []string {
	home, _ := os.UserHomeDir()
	wd, _ := os.Getwd()

	var installed []string
	for _, name := range agent.Names() {
		if skill.IsAgentInstalled(name, true, home, wd) ||
			skill.IsAgentInstalled(name, false, home, wd) {
			installed = append(installed, name)
		}
	}
	return installed
}

// copyProps returns a shallow copy of the given map.
func copyProps(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// telemetryStatePath returns the path to the telemetry milestone state file,
// respecting XDG_CONFIG_HOME on darwin.
func telemetryStatePath() string {
	cfgPath := config.UserConfigFilePath()
	return filepath.Join(filepath.Dir(cfgPath), ".telemetry-state")
}

// loadMilestoneState reads the milestone state from disk. Returns a zero-value
// milestoneState if the file does not exist or is invalid.
func loadMilestoneState(path string) milestoneState {
	var ms milestoneState
	data, err := os.ReadFile(path)
	if err != nil {
		return ms
	}
	_ = json.Unmarshal(data, &ms)
	return ms
}

// saveMilestoneState writes the milestone state to disk.
func saveMilestoneState(path string, ms milestoneState) {
	data, err := json.Marshal(ms)
	if err != nil {
		slog.Warn("failed to marshal milestone state", "err", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Warn("failed to create milestone state directory", "err", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		slog.Warn("failed to write milestone state", "err", err)
	}
}
