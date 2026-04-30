package ops

import (
	"testing"

	"gaal/internal/engine/render"
)

func TestPlanRepos_NotCloned(t *testing.T) {
	entries := []render.RepoEntry{
		{Path: "/repos/foo", Type: "git", Status: render.StatusNotCloned, URL: "https://example.com/foo.git"},
	}
	result := planRepos(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].Action != render.PlanClone {
		t.Errorf("expected action %q, got %q", render.PlanClone, result[0].Action)
	}
}

func TestPlanRepos_Dirty(t *testing.T) {
	entries := []render.RepoEntry{
		{Path: "/repos/foo", Type: "git", Status: render.StatusDirty, Current: "abc", Want: "main"},
	}
	result := planRepos(entries)
	if result[0].Action != render.PlanUpdate {
		t.Errorf("expected action %q, got %q", render.PlanUpdate, result[0].Action)
	}
}

func TestPlanRepos_OK(t *testing.T) {
	entries := []render.RepoEntry{
		{Path: "/repos/foo", Type: "git", Status: render.StatusOK, Current: "main", Want: "main"},
	}
	result := planRepos(entries)
	if result[0].Action != render.PlanNoOp {
		t.Errorf("expected action %q, got %q", render.PlanNoOp, result[0].Action)
	}
}

func TestPlanRepos_Error(t *testing.T) {
	entries := []render.RepoEntry{
		{Path: "/repos/foo", Type: "unknown", Status: render.StatusError, Error: "unsupported type"},
	}
	result := planRepos(entries)
	if result[0].Action != render.PlanError {
		t.Errorf("expected action %q, got %q", render.PlanError, result[0].Action)
	}
	if result[0].Error != "unsupported type" {
		t.Errorf("expected error %q, got %q", "unsupported type", result[0].Error)
	}
}

func TestPlanRepos_UnmanagedSkipped(t *testing.T) {
	entries := []render.RepoEntry{
		{Path: "/repos/foo", Type: "git", Status: render.StatusOK, Current: "main", Want: "main"},
		{Path: "/elsewhere/unrelated", Type: "git", Status: render.StatusUnmanaged},
	}
	result := planRepos(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry (unmanaged dropped), got %d: %+v", len(result), result)
	}
	if result[0].Path != "/repos/foo" {
		t.Errorf("expected only the config-declared repo to remain, got %q", result[0].Path)
	}
}

func TestPlanSkills_Partial(t *testing.T) {
	entries := []render.SkillEntry{
		{Source: "owner/repo", Agent: "claude-code", Status: render.StatusPartial, Missing: []string{"skill-a"}},
	}
	result := planSkills(entries)
	if result[0].Action != render.PlanCreate {
		t.Errorf("expected action %q, got %q", render.PlanCreate, result[0].Action)
	}
	if len(result[0].Install) != 1 || result[0].Install[0] != "skill-a" {
		t.Errorf("expected install=[skill-a], got %v", result[0].Install)
	}
}

func TestPlanSkills_Dirty(t *testing.T) {
	entries := []render.SkillEntry{
		{Source: "owner/repo", Agent: "claude-code", Status: render.StatusDirty, Modified: []string{"skill-b"}},
	}
	result := planSkills(entries)
	if result[0].Action != render.PlanUpdate {
		t.Errorf("expected action %q, got %q", render.PlanUpdate, result[0].Action)
	}
}

func TestPlanSkills_OK(t *testing.T) {
	entries := []render.SkillEntry{
		{Source: "owner/repo", Agent: "claude-code", Status: render.StatusOK, Installed: []string{"skill-a"}},
	}
	result := planSkills(entries)
	if result[0].Action != render.PlanNoOp {
		t.Errorf("expected action %q, got %q", render.PlanNoOp, result[0].Action)
	}
}

func TestPlanSkills_UnmanagedSkipped(t *testing.T) {
	entries := []render.SkillEntry{
		{Source: "owner/repo", Agent: "claude-code", Status: render.StatusOK, Installed: []string{"skill-a"}},
		{Source: "/elsewhere/skills", Agent: "claude-code", Status: render.StatusUnmanaged, Installed: []string{"skill-x"}},
	}
	result := planSkills(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry (unmanaged dropped), got %d: %+v", len(result), result)
	}
	if result[0].Source != "owner/repo" {
		t.Errorf("expected only the config-declared skill to remain, got %q", result[0].Source)
	}
}

func TestPlanMCPs_Absent(t *testing.T) {
	entries := []render.MCPEntry{
		{Name: "test-mcp", Target: "/path/to/config.json", Status: render.StatusAbsent},
	}
	result := planMCPs(entries)
	if result[0].Action != render.PlanCreate {
		t.Errorf("expected action %q, got %q", render.PlanCreate, result[0].Action)
	}
}

func TestPlanMCPs_Dirty(t *testing.T) {
	entries := []render.MCPEntry{
		{Name: "test-mcp", Target: "/path/to/config.json", Status: render.StatusDirty, Dirty: true},
	}
	result := planMCPs(entries)
	if result[0].Action != render.PlanUpdate {
		t.Errorf("expected action %q, got %q", render.PlanUpdate, result[0].Action)
	}
}

func TestPlanMCPs_Present(t *testing.T) {
	entries := []render.MCPEntry{
		{Name: "test-mcp", Target: "/path/to/config.json", Status: render.StatusPresent},
	}
	result := planMCPs(entries)
	if result[0].Action != render.PlanNoOp {
		t.Errorf("expected action %q, got %q", render.PlanNoOp, result[0].Action)
	}
}

func TestPlanMCPs_UnmanagedSkipped(t *testing.T) {
	entries := []render.MCPEntry{
		{Name: "test-mcp", Target: "/path/to/config.json", Status: render.StatusPresent},
		{Name: "stray-mcp", Target: "/elsewhere/mcp.json", Status: render.StatusUnmanaged},
	}
	result := planMCPs(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry (unmanaged dropped), got %d: %+v", len(result), result)
	}
	if result[0].Name != "test-mcp" {
		t.Errorf("expected only the config-declared MCP to remain, got %q", result[0].Name)
	}
}
