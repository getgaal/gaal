package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gaal/internal/engine/render"
)

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

// ── Audit — empty environment ────────────────────────────────────────────────

func TestAudit_EmptyEnv_Table(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	out := captureStdout(t, func() {
		if err := Audit(context.Background(), home, workDir, render.FormatTable); err != nil {
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
	home := t.TempDir()
	workDir := t.TempDir()
	out := captureStdout(t, func() {
		if err := Audit(context.Background(), home, workDir, render.FormatJSON); err != nil {
			t.Errorf("Audit json (empty env): %v", err)
		}
	})
	var report render.AuditReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, out)
	}
	_ = report.Skills
	_ = report.MCPs
}

// ── Audit — project skills discovery ────────────────────────────────────────

func TestAudit_ProjectSkills_Table(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	makeSkill(t, workDir, ".agents/skills", "my-skill", "A test skill")

	out := captureStdout(t, func() {
		if err := Audit(context.Background(), home, workDir, render.FormatTable); err != nil {
			t.Errorf("Audit table (project skills): %v", err)
		}
	})
	if !strings.Contains(out, "my-skill") {
		t.Errorf("output missing 'my-skill':\n%s", out)
	}
}

func TestAudit_ProjectSkills_JSON(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	makeSkill(t, workDir, ".agents/skills", "json-skill", "JSON test skill")

	out := captureStdout(t, func() {
		if err := Audit(context.Background(), home, workDir, render.FormatJSON); err != nil {
			t.Errorf("Audit json (project skills): %v", err)
		}
	})
	var report render.AuditReport
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

func TestAudit_TwoPass_NoProjectDuplicate(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	makeSkill(t, workDir, ".agents/skills", "shared-skill", "Shared skill")

	out := captureStdout(t, func() {
		if err := Audit(context.Background(), home, workDir, render.FormatJSON); err != nil {
			t.Errorf("Audit json (dedup): %v", err)
		}
	})
	var report render.AuditReport
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

func TestAudit_TwoPass_CanonicalAgent(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	makeSkill(t, workDir, ".claude/skills", "claude-skill", "Claude canonical skill")

	out := captureStdout(t, func() {
		if err := Audit(context.Background(), home, workDir, render.FormatJSON); err != nil {
			t.Errorf("Audit json (canonical agent): %v", err)
		}
	})
	var report render.AuditReport
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
