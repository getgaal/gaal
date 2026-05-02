package render

import (
	"strings"
	"testing"
)

func TestSummaryRenderer_AllHealthy(t *testing.T) {
	r := &StatusReport{
		Repositories: []RepoEntry{
			{Path: "src/a", Status: StatusOK},
			{Path: "src/b", Status: StatusOK},
			{Path: "src/c", Status: StatusDirty, Dirty: true},
		},
		Skills: []SkillEntry{
			{
				Source:    "github.com/org/skills",
				Agent:     "claude-code",
				Status:    StatusOK,
				Installed: []string{"skill-a", "skill-b", "skill-c"},
				Missing:   []string{},
			},
			{
				Source:    "github.com/org/skills",
				Agent:     "claude-desktop",
				Status:    StatusOK,
				Installed: []string{"skill-a", "skill-b", "skill-d"},
				Missing:   []string{},
			},
		},
		MCPs: []MCPEntry{
			{Name: "mcp-a", Status: StatusOK},
			{Name: "mcp-b", Status: StatusPresent},
			{Name: "mcp-c", Status: StatusDirty},
			{Name: "mcp-d", Status: StatusOK},
		},
		Agents: []AgentEntry{
			{Name: "claude-code", Installed: true},
			{Name: "claude-desktop", Installed: true},
			{Name: "codex", Installed: true},
		},
	}

	var buf strings.Builder
	sr := &summaryRenderer{}
	if err := sr.Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	out := buf.String()

	// 3 repos, 1 dirty
	if !strings.Contains(out, "3/3 repos cloned") {
		t.Errorf("expected '3/3 repos cloned', got:\n%s", out)
	}
	if !strings.Contains(out, "(1 dirty)") {
		t.Errorf("expected '(1 dirty)', got:\n%s", out)
	}
	// 4 unique skills across entries (skill-a, skill-b, skill-c, skill-d)
	if !strings.Contains(out, "4/4 skills installed") {
		t.Errorf("expected '4/4 skills installed', got:\n%s", out)
	}
	// 4 MCPs
	if !strings.Contains(out, "4/4 MCP servers registered") {
		t.Errorf("expected '4/4 MCP servers registered', got:\n%s", out)
	}
	// 3 agents
	if !strings.Contains(out, "3 agents configured") {
		t.Errorf("expected '3 agents configured', got:\n%s", out)
	}
	// 0 drift (dirty repo counts as cloned so no drift; dirty MCP still registered)
	if !strings.Contains(out, "0 drift") {
		t.Errorf("expected '0 drift', got:\n%s", out)
	}
	// All sections use ✓
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "repos") || strings.Contains(line, "skills") ||
			strings.Contains(line, "MCP") || strings.Contains(line, "agents") {
			if !strings.HasPrefix(line, "✓") {
				t.Errorf("expected ✓ prefix on line %q", line)
			}
		}
	}
}

func TestSummaryRenderer_WithErrors(t *testing.T) {
	r := &StatusReport{
		Repositories: []RepoEntry{
			{Path: "src/a", Status: StatusOK},
			{Path: "src/b", Status: StatusNotCloned},
		},
		Skills: []SkillEntry{
			{
				Source:    "github.com/org/skills",
				Agent:     "claude-code",
				Status:    StatusPartial,
				Installed: []string{"skill-a", "skill-b"},
				Missing:   []string{"skill-c", "skill-d"},
			},
			{
				// Unmanaged entry should be ignored
				Source: "other",
				Agent:  "claude-code",
				Status: StatusUnmanaged,
			},
		},
		MCPs: []MCPEntry{
			{Name: "mcp-a", Status: StatusOK},
			{Name: "mcp-b", Status: StatusAbsent},
			{Name: "mcp-c", Status: StatusUnmanaged},
		},
		Agents: []AgentEntry{
			{Name: "claude-code", Installed: true},
			{Name: "claude-desktop", Installed: false},
		},
	}

	var buf strings.Builder
	sr := &summaryRenderer{}
	if err := sr.Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	out := buf.String()

	// 1 of 2 repos cloned
	if !strings.Contains(out, "1/2 repos cloned") {
		t.Errorf("expected '1/2 repos cloned', got:\n%s", out)
	}
	// 2 installed, 4 total unique skills
	if !strings.Contains(out, "2/4 skills installed") {
		t.Errorf("expected '2/4 skills installed', got:\n%s", out)
	}
	// 1 of 2 non-unmanaged MCPs registered
	if !strings.Contains(out, "1/2 MCP servers registered") {
		t.Errorf("expected '1/2 MCP servers registered', got:\n%s", out)
	}
	// 1 installed agent
	if !strings.Contains(out, "1 agents configured") {
		t.Errorf("expected '1 agents configured', got:\n%s", out)
	}
	// Drift > 0
	if strings.Contains(out, "0 drift") {
		t.Errorf("expected non-zero drift, got:\n%s", out)
	}
	// Sections with issues use !
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "skills") {
			if !strings.HasPrefix(line, "!") {
				t.Errorf("expected ! prefix on skills line %q", line)
			}
		}
		if strings.Contains(line, "MCP") {
			if !strings.HasPrefix(line, "!") {
				t.Errorf("expected ! prefix on MCP line %q", line)
			}
		}
	}
}

func TestSummaryRenderer_Empty(t *testing.T) {
	r := &StatusReport{}

	var buf strings.Builder
	sr := &summaryRenderer{}
	if err := sr.Render(&buf, r); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "no managed resources" {
		t.Errorf("expected 'no managed resources', got %q", out)
	}
}
