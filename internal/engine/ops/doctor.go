package ops

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gaal/internal/config"
	"gaal/internal/core/agent"
	"gaal/internal/skill"
	"gaal/internal/telemetry"
)

// Severity indicates the importance level of a doctor finding.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Finding represents a single check result from the doctor command.
type Finding struct {
	Section  string   `json:"section"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// DoctorReport is the structured output of the doctor check pipeline.
type DoctorReport struct {
	Findings []Finding `json:"findings"`
	ExitCode int       `json:"exit_code"`
}

// DoctorOptions configures the doctor check behaviour.
type DoctorOptions struct {
	Offline bool
}

// RunDoctor executes all sanity checks against the given config and returns
// a structured report. Exit code: 0 = clean, 1 = warnings only, 2 = any errors.
func RunDoctor(cfg *config.Config, opts DoctorOptions) *DoctorReport {
	slog.Debug("running doctor checks", "offline", opts.Offline)

	var findings []Finding
	findings = append(findings, checkVersion(cfg)...)
	findings = append(findings, checkTelemetry(cfg)...)
	findings = append(findings, checkSkillSources(cfg, opts.Offline)...)
	findings = append(findings, checkMCPTargets(cfg)...)
	findings = append(findings, checkAgents()...)

	exitCode := 0
	for _, f := range findings {
		switch f.Severity {
		case SeverityError:
			exitCode = 2
		case SeverityWarning:
			if exitCode < 1 {
				exitCode = 1
			}
		}
	}

	return &DoctorReport{
		Findings: findings,
		ExitCode: exitCode,
	}
}

// checkVersion reports the schema version status.
func checkVersion(cfg *config.Config) []Finding {
	if cfg.Version == nil {
		return []Finding{
			{Section: "config", Severity: SeverityWarning, Message: "config is missing 'version: 1'; this will be required in a future release"},
		}
	}
	return []Finding{
		{Section: "config", Severity: SeverityInfo, Message: fmt.Sprintf("version: %d", *cfg.Version)},
	}
}

// checkTelemetry reports the current telemetry state as an info finding.
func checkTelemetry(cfg *config.Config) []Finding {
	status, source := telemetry.Status(cfg.Telemetry)
	msg := fmt.Sprintf("telemetry: %s", status)
	if source != "" {
		msg += fmt.Sprintf(" (%s)", source)
	}
	return []Finding{
		{Section: "telemetry", Severity: SeverityInfo, Message: msg},
	}
}

// checkSkillSources validates each configured skill source.
func checkSkillSources(cfg *config.Config, offline bool) []Finding {
	var findings []Finding

	// Check for duplicate sources.
	seen := make(map[string]int, len(cfg.Skills))
	for _, sk := range cfg.Skills {
		seen[sk.Source]++
	}
	for src, count := range seen {
		if count > 1 {
			findings = append(findings, Finding{
				Section:  "skills",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("duplicate skill source %q (appears %d times)", src, count),
			})
		}
	}

	for _, sk := range cfg.Skills {
		// Warn on agents:["*"] with zero detected agents.
		if len(sk.Agents) == 1 && sk.Agents[0] == "*" {
			if countInstalledAgents() == 0 {
				findings = append(findings, Finding{
					Section:  "skills",
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("skill %q targets agents:[\"*\"] but no agents are detected", sk.Source),
				})
			}
		}

		if isRemoteSource(sk.Source) {
			// Remote source (URL or GitHub shorthand).
			if offline {
				findings = append(findings, Finding{
					Section:  "skills",
					Severity: SeverityInfo,
					Message:  fmt.Sprintf("skipped reachability check for %q (offline mode)", sk.Source),
				})
			} else {
				url := resolveSkillURL(sk.Source)
				if err := checkRemoteReachable(url); err != nil {
					findings = append(findings, Finding{
						Section:  "skills",
						Severity: SeverityError,
						Message:  fmt.Sprintf("remote skill source unreachable: %s (%v)", url, err),
					})
				}
			}
		} else {
			// Local path.
			if _, err := os.Stat(sk.Source); err != nil {
				findings = append(findings, Finding{
					Section:  "skills",
					Severity: SeverityError,
					Message:  fmt.Sprintf("local skill path does not exist: %s", sk.Source),
				})
			}
		}
	}

	return findings
}

// checkMCPTargets validates each configured MCP target file.
func checkMCPTargets(cfg *config.Config) []Finding {
	var findings []Finding

	home, _ := os.UserHomeDir()

	for _, m := range cfg.MCPs {
		// Warn if target is outside $HOME.
		if home != "" && !strings.HasPrefix(m.Target, home+string(filepath.Separator)) && m.Target != home {
			findings = append(findings, Finding{
				Section:  "mcps",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("MCP %q target %q is outside $HOME", m.Name, m.Target),
			})
		}

		data, err := os.ReadFile(m.Target)
		if err != nil {
			if os.IsNotExist(err) {
				// Target doesn't exist yet — this is normal for first sync.
				findings = append(findings, Finding{
					Section:  "mcps",
					Severity: SeverityInfo,
					Message:  fmt.Sprintf("MCP %q target does not exist yet: %s (will be created on sync)", m.Name, m.Target),
				})
			} else {
				findings = append(findings, Finding{
					Section:  "mcps",
					Severity: SeverityError,
					Message:  fmt.Sprintf("MCP %q target unreadable: %v", m.Name, err),
				})
			}
			continue
		}

		// Check if target is valid JSON.
		if !json.Valid(data) {
			findings = append(findings, Finding{
				Section:  "mcps",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("MCP %q target contains invalid JSON: %s", m.Name, m.Target),
			})
		}
	}

	return findings
}

// checkAgents counts installed agents and reports as info.
func checkAgents() []Finding {
	count := countInstalledAgents()
	return []Finding{
		{Section: "agents", Severity: SeverityInfo, Message: fmt.Sprintf("%d agent(s) detected", count)},
	}
}

// isRemoteSource returns true for URLs and GitHub shorthands.
func isRemoteSource(source string) bool {
	if strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasPrefix(source, "ssh://") {
		return true
	}
	// GitHub shorthand: exactly owner/repo — no scheme, no dots in owner.
	parts := strings.Split(source, "/")
	if len(parts) == 2 && !strings.Contains(parts[0], ".") &&
		!strings.HasPrefix(source, ".") && !strings.HasPrefix(source, "~") {
		return true
	}
	return false
}

// resolveSkillURL returns an HTTPS URL for a reachability check.
// Handles GitHub shorthands (owner/repo), git@ SSH URLs, and plain HTTPS URLs.
func resolveSkillURL(source string) string {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return source
	}
	// git@github.com:owner/repo.git → https://github.com/owner/repo
	if strings.HasPrefix(source, "git@") {
		// git@host:owner/repo.git
		s := strings.TrimPrefix(source, "git@")
		s = strings.TrimSuffix(s, ".git")
		s = strings.Replace(s, ":", "/", 1)
		return "https://" + s
	}
	// ssh://git@github.com/owner/repo.git
	if strings.HasPrefix(source, "ssh://") {
		s := strings.TrimPrefix(source, "ssh://")
		s = strings.TrimPrefix(s, "git@")
		s = strings.TrimSuffix(s, ".git")
		return "https://" + s
	}
	// GitHub shorthand: owner/repo
	return "https://github.com/" + source
}

// checkRemoteReachable performs an HTTP HEAD request with a 5-second timeout
// and returns an error if the status code is >= 400.
func checkRemoteReachable(url string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Head(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// countInstalledAgents returns the number of agents currently installed on this
// machine, using agent.Names() and skill.IsAgentInstalled().
func countInstalledAgents() int {
	home, _ := os.UserHomeDir()
	wd, _ := os.Getwd()

	count := 0
	for _, name := range agent.Names() {
		if skill.IsAgentInstalled(name, true, home, wd) ||
			skill.IsAgentInstalled(name, false, home, wd) {
			count++
		}
	}
	return count
}
