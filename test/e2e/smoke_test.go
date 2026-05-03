//go:build e2e

// Smoke tests for the seven user-facing commands the rest of the e2e suite
// did not cover before #144: init, migrate, doctor, agents, info, version,
// schema. Each test asserts only the documented contract — exit code, the
// shape of stdout, and any side effect declared in --help — so they pin the
// behavior without coupling to incidental output.
package e2e

import (
	"encoding/json"
	"path"
	"strings"
	"testing"
)

// ── version / schema ────────────────────────────────────────────────────────

// TestSmoke_Version exercises the trivial reporting command. It must exit 0
// and print something non-empty containing "gaal".
func TestSmoke_Version(t *testing.T) {
	env := newTestEnv(t)
	res := env.mustGaal(t, "", "version")
	if !strings.Contains(strings.ToLower(res.Stdout+res.Stderr), "gaal") {
		t.Errorf("expected `gaal version` output to mention gaal\n%s", res.Combined())
	}
}

// TestSmoke_Schema asserts that `gaal schema` emits a parseable JSON Schema
// document on stdout. Exit code MUST be 0 and the body MUST decode as JSON
// with a top-level "$schema" or "type" key (one of the standard JSON Schema
// markers).
func TestSmoke_Schema(t *testing.T) {
	env := newTestEnv(t)
	res := env.mustGaal(t, "", "schema")
	var doc map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &doc); err != nil {
		t.Fatalf("schema output is not valid JSON: %v\n%s", err, res.Stdout)
	}
	if _, hasSchema := doc["$schema"]; !hasSchema {
		if _, hasType := doc["type"]; !hasType {
			t.Errorf("schema doc missing both $schema and type top-level keys: %v", doc)
		}
	}
}

// ── agents (no config, no network) ─────────────────────────────────────────

// TestSmoke_Agents lists the agent registry. The command takes no args and
// must succeed with at least one row in the body — gaal ships with a
// non-empty registry.
func TestSmoke_Agents(t *testing.T) {
	env := newTestEnv(t)
	res := env.mustGaal(t, "", "agents")
	if strings.TrimSpace(res.Stdout) == "" {
		t.Errorf("expected `gaal agents` to produce non-empty stdout\n%s", res.Combined())
	}
	// At least one of the well-known agents must appear; pin claude-code
	// since it is the project's first-class target.
	if !strings.Contains(res.Stdout, "claude-code") {
		t.Errorf("expected agent registry to include claude-code\n%s", res.Stdout)
	}
}

// TestSmoke_AgentsInstalled exercises the -i flag. The fresh test HOME has
// no agents installed, so the output should still be valid (zero rows OK)
// and exit 0.
func TestSmoke_AgentsInstalled(t *testing.T) {
	env := newTestEnv(t)
	res := env.mustGaal(t, "", "agents", "-i")
	_ = res // No assertion beyond exit 0 — empty list is valid.
}

// ── info ────────────────────────────────────────────────────────────────────

// TestSmoke_InfoAgent shows the detail view for a known agent. The command
// is `gaal info <kind> [name]` per cmd/info.go; here we ask for an agent.
func TestSmoke_InfoAgent(t *testing.T) {
	env := newTestEnv(t)
	res := env.mustGaal(t, "", "info", "agent", "claude-code")
	if !strings.Contains(res.Stdout, "claude-code") {
		t.Errorf("expected `gaal info agent claude-code` to mention the agent\n%s",
			res.Combined())
	}
}

// ── doctor ──────────────────────────────────────────────────────────────────

// TestSmoke_DoctorOffline runs the diagnostic with --offline so no network
// is touched. Exit code is allowed to be 0 (clean), 1 (warnings) or 2
// (errors) — the test asserts the contract is one of those, not which one.
func TestSmoke_DoctorOffline(t *testing.T) {
	env := newTestEnv(t)
	cfg := newConfig().String() // Empty config — doctor must still run.
	cfgPath := env.writeProjectConfig(t, cfg)

	res := env.gaal(t, cfgPath, "doctor", "--offline", "--no-upsell")
	switch res.ExitCode {
	case 0, 1, 2:
		// expected
	default:
		t.Fatalf("doctor --offline returned unexpected exit %d\n%s",
			res.ExitCode, res.Combined())
	}
}

// TestSmoke_DoctorJSON verifies the JSON output renderer. Output must
// decode and contain the documented "exit_code" and "findings" keys.
func TestSmoke_DoctorJSON(t *testing.T) {
	env := newTestEnv(t)
	cfgPath := env.writeProjectConfig(t, newConfig().String())

	res := env.gaal(t, cfgPath, "doctor", "--offline", "--no-upsell", "-o", "json")
	if res.ExitCode != 0 && res.ExitCode != 1 && res.ExitCode != 2 {
		t.Fatalf("doctor json: unexpected exit %d\n%s", res.ExitCode, res.Combined())
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &report); err != nil {
		t.Fatalf("doctor json output is not valid JSON: %v\n%s", err, res.Stdout)
	}
	for _, key := range []string{"findings", "exit_code"} {
		if _, ok := report[key]; !ok {
			t.Errorf("doctor JSON missing %q key: %v", key, report)
		}
	}
}

// ── init ────────────────────────────────────────────────────────────────────

// TestSmoke_InitEmptyProject writes a fresh skeleton project config and
// confirms a subsequent `sync` against it succeeds (the skeleton must be
// valid YAML that parses).
func TestSmoke_InitEmptyProject(t *testing.T) {
	env := newTestEnv(t)
	res := env.mustGaal(t, "", "init", "--empty", "--scope", "project", "--force")
	_ = res
	cfgPath := path.Join(env.workdir, "gaal.yaml")
	if !env.c.FileExists(t, cfgPath) {
		t.Fatalf("init did not create %s", cfgPath)
	}
	// The generated skeleton must round-trip through sync without error.
	env.mustGaal(t, cfgPath, "sync")
}

// TestSmoke_InitImportAllGlobal exercises the import flow non-interactively
// in global scope. The fresh HOME has no agents installed, so the import
// candidate set may be empty — but the command must still exit 0 and write
// the global config file.
func TestSmoke_InitImportAllGlobal(t *testing.T) {
	env := newTestEnv(t)
	env.mustGaal(t, "", "init", "--import-all", "--scope", "global", "--force")
	cfgPath := path.Join(env.home, ".config", "gaal", "config.yaml")
	if !env.c.FileExists(t, cfgPath) {
		t.Fatalf("init --scope global did not create %s", cfgPath)
	}
}

// ── migrate (stub) ──────────────────────────────────────────────────────────

// TestSmoke_MigrateDryRun exercises the migrate stub. The command currently
// only prints a disclaimer (community edition is not yet shipping) but must
// still parse its flags + URL and exit 0 in dry-run mode.
func TestSmoke_MigrateDryRun(t *testing.T) {
	env := newTestEnv(t)
	cfgPath := env.writeProjectConfig(t, newConfig().String())
	res := env.mustGaal(t, cfgPath, "migrate", "--to", "community",
		"--dry-run", "https://example.com/community")
	combined := res.Combined()
	// Disclaimer text should mention "not yet" or "community" — both come
	// from cmd/migrate.go's printDisclaimer.
	if !strings.Contains(combined, "community") {
		t.Errorf("expected migrate output to reference 'community' edition\n%s", combined)
	}
}
