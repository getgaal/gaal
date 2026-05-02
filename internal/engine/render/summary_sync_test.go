package render

import (
	"strings"
	"testing"
	"time"
)

func TestRenderSyncBrief_WithChanges(t *testing.T) {
	plan := &PlanReport{
		HasChanges: true,
		Repositories: []PlanRepoEntry{
			{Path: "src/example", Action: PlanClone},
		},
		Skills: []PlanSkillEntry{
			{
				Source:  "github.com/org/skills",
				Agent:   "claude-code",
				Action:  PlanCreate,
				Install: []string{"code-review", "refactor"},
			},
		},
		MCPs: []PlanMCPEntry{
			{Name: "memory-mcp", Target: "~/.config/claude/claude_desktop_config.json", Action: PlanUpdate},
		},
	}
	status := &StatusReport{
		Skills: []SkillEntry{
			{
				Source:    "github.com/org/skills",
				Agent:     "claude-code",
				Status:    StatusOK,
				Installed: []string{"code-review", "refactor"},
			},
			{
				Source:    "github.com/org/skills",
				Agent:     "cursor",
				Status:    StatusOK,
				Installed: []string{"code-review", "refactor"},
			},
		},
		MCPs: []MCPEntry{
			{Name: "memory-mcp", Status: StatusOK, Target: "~/.config/claude/claude_desktop_config.json"},
		},
		Agents: []AgentEntry{
			{Name: "claude-code", Installed: true},
			{Name: "cursor", Installed: true},
		},
	}

	var buf strings.Builder
	err := RenderSyncBrief(&buf, plan, status, 1200*time.Millisecond)
	if err != nil {
		t.Fatalf("RenderSyncBrief error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "Cloned src/example") {
		t.Errorf("expected 'Cloned src/example', got:\n%s", out)
	}
	if !strings.Contains(out, "Installed 2 new skill") {
		t.Errorf("expected 'Installed 2 new skill', got:\n%s", out)
	}
	if !strings.Contains(out, "Updated memory-mcp config") {
		t.Errorf("expected 'Updated memory-mcp config', got:\n%s", out)
	}
	if !strings.Contains(out, "→ claude-code:") {
		t.Errorf("expected '→ claude-code:' rollup line, got:\n%s", out)
	}
	if !strings.Contains(out, "→ cursor:") {
		t.Errorf("expected '→ cursor:' rollup line, got:\n%s", out)
	}
	if !strings.Contains(out, "✓ Synced in") {
		t.Errorf("expected '✓ Synced in', got:\n%s", out)
	}
	// Should NOT say "In sync" when there are changes
	if strings.Contains(out, "In sync") {
		t.Errorf("unexpected 'In sync' when changes exist, got:\n%s", out)
	}
}

func TestRenderSyncBrief_NoChanges(t *testing.T) {
	plan := &PlanReport{
		HasChanges: false,
	}
	status := &StatusReport{
		Skills: []SkillEntry{
			{
				Source:    "github.com/org/skills",
				Agent:     "claude-code",
				Status:    StatusOK,
				Installed: []string{"code-review", "refactor"},
			},
			{
				Source:    "github.com/org/skills",
				Agent:     "cursor",
				Status:    StatusOK,
				Installed: []string{"code-review", "refactor"},
			},
		},
		MCPs: []MCPEntry{
			{Name: "memory-mcp", Status: StatusOK, Target: "~/.config/claude/claude_desktop_config.json"},
		},
		Agents: []AgentEntry{
			{Name: "claude-code", Installed: true},
			{Name: "cursor", Installed: true},
		},
	}

	var buf strings.Builder
	err := RenderSyncBrief(&buf, plan, status, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("RenderSyncBrief error: %v", err)
	}
	out := buf.String()

	// No change lines expected
	if strings.Contains(out, "Cloned") {
		t.Errorf("unexpected 'Cloned' in no-change output, got:\n%s", out)
	}
	if strings.Contains(out, "Installed") {
		t.Errorf("unexpected 'Installed' in no-change output, got:\n%s", out)
	}
	if !strings.Contains(out, "→ claude-code:") {
		t.Errorf("expected '→ claude-code:' rollup line, got:\n%s", out)
	}
	if !strings.Contains(out, "→ cursor:") {
		t.Errorf("expected '→ cursor:' rollup line, got:\n%s", out)
	}
	if !strings.Contains(out, "✓ In sync") {
		t.Errorf("expected '✓ In sync', got:\n%s", out)
	}
	// Should NOT say "Synced in" when no changes
	if strings.Contains(out, "Synced in") {
		t.Errorf("unexpected 'Synced in' when no changes, got:\n%s", out)
	}
}

func TestRenderSyncBrief_NilInputs(t *testing.T) {
	var buf strings.Builder
	err := RenderSyncBrief(&buf, nil, nil, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("RenderSyncBrief(nil, nil) error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "✓ In sync") {
		t.Errorf("expected '✓ In sync' for nil inputs, got:\n%s", out)
	}
}
