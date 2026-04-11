package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pterm/pterm"

	"gaal/internal/config"
)

// ── Info (engine method) ──────────────────────────────────────────────────────

func TestInfo_UnknownPackage(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		err := e.Info(context.Background(), "unknown", "", FormatTable)
		if err == nil {
			t.Error("expected error for unknown package type")
		}
	})
	_ = out // output may be partial; we just care about the error
}

func TestInfo_UnknownPackage_Error(t *testing.T) {
	e := New(&config.Config{})
	err := e.Info(context.Background(), "foo", "", FormatTable)
	if err == nil || !strings.Contains(err.Error(), "unknown package type") {
		t.Errorf("expected 'unknown package type' error, got: %v", err)
	}
}

func TestInfo_JSON_Agent(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "agent", "", FormatJSON); err != nil {
			t.Errorf("Info agent json: %v", err)
		}
	})
	var result struct {
		Agents []AgentEntry `json:"agents"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, out)
	}
	if len(result.Agents) == 0 {
		t.Error("expected at least one agent in JSON output")
	}

	for _, agent := range result.Agents {
		if agent.Name != "cline" {
			continue
		}
		if agent.ProjectSkillsDir != ".agents/skills" {
			t.Errorf("cline project_skills_dir = %q, want .agents/skills", agent.ProjectSkillsDir)
		}
		if agent.GlobalSkillsDir == "" {
			t.Fatal("cline global_skills_dir is empty")
		}
		if !agent.ProjectSkillsViaGeneric {
			t.Error("cline project_skills_via_generic = false, want true")
		}
		if !agent.GlobalSkillsViaGeneric {
			t.Error("cline global_skills_via_generic = false, want true")
		}
		return
	}

	t.Fatal("cline agent not found in JSON output")
}

func TestInfo_JSON_Agent_Filter(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "agent", "cursor", FormatJSON); err != nil {
			t.Errorf("Info agent json filter: %v", err)
		}
	})
	var result struct {
		Agents []AgentEntry `json:"agents"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, a := range result.Agents {
		if !strings.Contains(a.Name, "cursor") {
			t.Errorf("expected only cursor agents, got %q", a.Name)
		}
	}
}

func TestInfo_JSON_Repo_Empty(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "repo", "", FormatJSON); err != nil {
			t.Errorf("Info repo json: %v", err)
		}
	})
	var result struct {
		Repositories []RepoEntry `json:"repositories"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
}

func TestInfo_JSON_UnknownPackage(t *testing.T) {
	e := New(&config.Config{})
	err := e.Info(context.Background(), "bad", "", FormatJSON)
	if err == nil || !strings.Contains(err.Error(), "unknown package type") {
		t.Errorf("expected 'unknown package type' error in JSON mode, got: %v", err)
	}
}

func TestInfo_Repo_Empty(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "repo", "", FormatTable); err != nil {
			t.Errorf("Info repo empty: %v", err)
		}
	})
	if !strings.Contains(out, "Repositories") {
		t.Errorf("output missing 'Repositories' section, got:\n%s", out)
	}
}

func TestInfo_Skill_Empty(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "skill", "", FormatTable); err != nil {
			t.Errorf("Info skill empty: %v", err)
		}
	})
	if !strings.Contains(out, "Skills") {
		t.Errorf("output missing 'Skills' section, got:\n%s", out)
	}
}

func TestInfo_MCP_Empty(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "mcp", "", FormatTable); err != nil {
			t.Errorf("Info mcp empty: %v", err)
		}
	})
	if !strings.Contains(out, "MCP") {
		t.Errorf("output missing 'MCP' section, got:\n%s", out)
	}
}

func TestInfo_Agent_NoFilter(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "agent", "", FormatTable); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	clean := pterm.RemoveColorFromString(out)
	if !strings.Contains(clean, "Supported Agents") {
		t.Error("expected 'Supported Agents' section header")
	}
}

func TestInfo_Agent_Filter(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "agent", "goose", FormatTable); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	clean := pterm.RemoveColorFromString(out)
	if !strings.Contains(clean, "goose") {
		t.Error("expected 'goose' in filtered agent output")
	}
}
