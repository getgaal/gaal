package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gaal/internal/config"
)

// ── Audit — empty environment ────────────────────────────────────────────────

func TestAudit_EmptyEnv_Table(t *testing.T) {
	e := NewWithOptions(nil_config(t), Options{WorkDir: t.TempDir()})
	out := captureStdout(t, func() {
		if err := e.Audit(context.Background(), FormatTable); err != nil {
			t.Errorf("Audit table (empty env): %v", err)
		}
	})
	if !strings.Contains(out, "Discovered Skills") {
		t.Errorf("output missing 'Discovered Skills' section:\n%s", out)
	}
	if !strings.Contains(out, "Discovered MCP Servers") {
		t.Errorf("output missing 'Discovered MCP Servers' section:\n%s", out)
	}
}

func TestAudit_EmptyEnv_JSON(t *testing.T) {
	e := NewWithOptions(nil_config(t), Options{WorkDir: t.TempDir()})
	out := captureStdout(t, func() {
		if err := e.Audit(context.Background(), FormatJSON); err != nil {
			t.Errorf("Audit json (empty env): %v", err)
		}
	})
	var report AuditReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, out)
	}
	// Empty env: no skills or MCPs, but keys must be present (null is ok).
	// Just verify the JSON decoded without error and both slices are well-typed.
	_ = report.Skills
	_ = report.MCPs
}

// ── Audit — project skills discovery ────────────────────────────────────────

// makeSkill creates <workDir>/<relDir>/<skillName>/SKILL.md with optional
// frontmatter containing the given description.
func makeSkill(t *testing.T, workDir, relDir, skillName, desc string) {
	t.Helper()
	dir := filepath.Join(workDir, relDir, skillName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + skillName + "\ndescription: " + desc + "\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAudit_ProjectSkills_Table(t *testing.T) {
	workDir := t.TempDir()
	// .agents/skills is the canonical project_skills_dir for many agents (amp,
	// github-copilot, etc.). A skill placed here must be discovered.
	makeSkill(t, workDir, ".agents/skills", "my-skill", "A test skill")

	e := NewWithOptions(nil_config(t), Options{WorkDir: workDir})
	out := captureStdout(t, func() {
		if err := e.Audit(context.Background(), FormatTable); err != nil {
			t.Errorf("Audit table (project skills): %v", err)
		}
	})
	if !strings.Contains(out, "my-skill") {
		t.Errorf("output missing 'my-skill':\n%s", out)
	}
}

func TestAudit_ProjectSkills_JSON(t *testing.T) {
	workDir := t.TempDir()
	makeSkill(t, workDir, ".agents/skills", "json-skill", "JSON test skill")

	e := NewWithOptions(nil_config(t), Options{WorkDir: workDir})
	out := captureStdout(t, func() {
		if err := e.Audit(context.Background(), FormatJSON); err != nil {
			t.Errorf("Audit json (project skills): %v", err)
		}
	})
	var report AuditReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	found := false
	for _, s := range report.Skills {
		if s.Name == "json-skill" {
			found = true
			if s.Source != "project" {
				t.Errorf("expected source=project, got %q", s.Source)
			}
			break
		}
	}
	if !found {
		t.Errorf("'json-skill' not found in JSON skills list")
	}
}

// ── Audit — two-pass deduplication ──────────────────────────────────────────

// TestAudit_TwoPass_NoProjectDuplicate verifies that a skill placed in a
// shared directory (e.g. .agents/skills) appears only once even though
// multiple agents include that path in their search lists.
func TestAudit_TwoPass_NoProjectDuplicate(t *testing.T) {
	workDir := t.TempDir()
	makeSkill(t, workDir, ".agents/skills", "shared-skill", "Shared skill")

	e := NewWithOptions(nil_config(t), Options{WorkDir: workDir})
	out := captureStdout(t, func() {
		if err := e.Audit(context.Background(), FormatJSON); err != nil {
			t.Errorf("Audit json (dedup): %v", err)
		}
	})
	var report AuditReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	count := 0
	for _, s := range report.Skills {
		if s.Name == "shared-skill" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected shared-skill to appear exactly once, got %d", count)
	}
}

// TestAudit_TwoPass_CanonicalAgent verifies that a skill placed in an agent's
// canonical project directory is attributed to that agent rather than another
// agent that lists the same directory in its extended search list.
//
// .claude/skills is the canonical project_skills_dir for claude-code, but it
// also appears in the project_skills_search of amp, github-copilot, and others.
// After the two-pass fix, the skill must be attributed to claude-code.
func TestAudit_TwoPass_CanonicalAgent(t *testing.T) {
	workDir := t.TempDir()
	makeSkill(t, workDir, ".claude/skills", "claude-skill", "Claude canonical skill")

	e := NewWithOptions(nil_config(t), Options{WorkDir: workDir})
	out := captureStdout(t, func() {
		if err := e.Audit(context.Background(), FormatJSON); err != nil {
			t.Errorf("Audit json (canonical agent): %v", err)
		}
	})
	var report AuditReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	for _, s := range report.Skills {
		if s.Name == "claude-skill" {
			if s.Agent != "claude-code" {
				t.Errorf("expected agent=claude-code, got %q", s.Agent)
			}
			return
		}
	}
	t.Error("'claude-skill' not found in JSON skills list")
}

// ── helpers ──────────────────────────────────────────────────────────────────

// nil_config returns an empty *config.Config (kept as a named helper to keep
// test bodies concise without importing config).
func nil_config(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{}
}
