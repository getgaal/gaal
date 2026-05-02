package render

import (
	"strings"
	"testing"
)

func TestSummaryAudit_WithData(t *testing.T) {
	r := &AuditReport{
		Skills: []AuditSkillEntry{
			{Name: "code-review", Agent: "claude-code", Source: "project", Path: "/home/user/.claude/skills/code-review"},
			{Name: "code-review", Agent: "cursor", Source: "project", Path: "/home/user/.cursor/skills/code-review"},
			{Name: "commit", Agent: "claude-code", Source: "project", Path: "/home/user/.claude/skills/commit"},
			{Name: "summarise", Agent: "cursor", Source: "global", Path: "/home/user/.cursor/skills/summarise"},
		},
		MCPs: []AuditMCPEntry{
			{Agent: "claude-code", ConfigFile: "/home/user/.claude/claude_desktop_config.json", Servers: []string{"filesystem", "git"}},
			{Agent: "cursor", ConfigFile: "/home/user/.cursor/mcp.json", Servers: []string{"git", "github"}},
		},
	}

	var buf strings.Builder
	sr := &summaryAuditRenderer{}
	if err := sr.Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	out := buf.String()

	// 3 unique skill names: code-review, commit, summarise
	if !strings.Contains(out, "Found 3 skills") {
		t.Errorf("expected 'Found 3 skills', got:\n%s", out)
	}
	// 3 unique MCP servers: filesystem, git, github
	if !strings.Contains(out, "Found 3 MCP servers") {
		t.Errorf("expected 'Found 3 MCP servers', got:\n%s", out)
	}
	// 2 unique agents: claude-code, cursor
	if !strings.Contains(out, "2 agents scanned") {
		t.Errorf("expected '2 agents scanned', got:\n%s", out)
	}
	// Agent list sorted alphabetically
	if !strings.Contains(out, "claude-code, cursor") {
		t.Errorf("expected 'claude-code, cursor', got:\n%s", out)
	}
}

func TestSummaryAudit_WithData_SingleSkillAndAgent(t *testing.T) {
	r := &AuditReport{
		Skills: []AuditSkillEntry{
			{Name: "commit", Agent: "claude-code", Source: "project", Path: "/home/user/.claude/skills/commit"},
		},
		MCPs: []AuditMCPEntry{
			{Agent: "claude-code", ConfigFile: "/home/user/.claude/mcp.json", Servers: []string{"filesystem"}},
		},
	}

	var buf strings.Builder
	sr := &summaryAuditRenderer{}
	if err := sr.Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	out := buf.String()

	// Singular: "1 skill"
	if !strings.Contains(out, "Found 1 skill ") {
		t.Errorf("expected 'Found 1 skill ', got:\n%s", out)
	}
	// Singular: "1 MCP server"
	if !strings.Contains(out, "Found 1 MCP server") {
		t.Errorf("expected 'Found 1 MCP server', got:\n%s", out)
	}
	// Singular: "1 agent"
	if !strings.Contains(out, "1 agent scanned") {
		t.Errorf("expected '1 agent scanned', got:\n%s", out)
	}
}

func TestSummaryAudit_Empty(t *testing.T) {
	r := &AuditReport{}

	var buf strings.Builder
	sr := &summaryAuditRenderer{}
	if err := sr.Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "No skills or MCP servers discovered." {
		t.Errorf("expected 'No skills or MCP servers discovered.', got %q", out)
	}
}
