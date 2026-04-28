package ops

import (
	"reflect"
	"testing"

	"gaal/internal/config"
)

func TestBuildPlan_GlobalFlagMatchesScope(t *testing.T) {
	cand := Candidate{
		Kind:        CandidateSkill,
		AgentName:   "claude-code",
		SkillName:   "frontend-design",
		SkillSource: "anthropics/skills",
	}
	plan := BuildPlan([]Candidate{cand}, ScopeGlobal)
	if len(plan.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(plan.Skills))
	}
	if !plan.Skills[0].Global {
		t.Error("Global should be true for ScopeGlobal")
	}

	plan = BuildPlan([]Candidate{cand}, ScopeProject)
	if plan.Skills[0].Global {
		t.Error("Global should be false for ScopeProject")
	}
}

func TestBuildPlan_GroupSkillsBySourceAndAgent(t *testing.T) {
	cands := []Candidate{
		{Kind: CandidateSkill, AgentName: "claude-code", SkillName: "b", SkillSource: "anthropics/skills"},
		{Kind: CandidateSkill, AgentName: "claude-code", SkillName: "a", SkillSource: "anthropics/skills"},
		{Kind: CandidateSkill, AgentName: "claude-code", SkillName: "c", SkillSource: "anthropics/skills"},
	}
	plan := BuildPlan(cands, ScopeProject)
	if len(plan.Skills) != 1 {
		t.Fatalf("expected 1 grouped skill entry, got %d", len(plan.Skills))
	}
	got := plan.Skills[0]
	if got.Source != "anthropics/skills" {
		t.Errorf("source: got %q", got.Source)
	}
	if !reflect.DeepEqual(got.Agents, []string{"claude-code"}) {
		t.Errorf("agents: got %v", got.Agents)
	}
	if !reflect.DeepEqual(got.Select, []string{"a", "b", "c"}) {
		t.Errorf("select: got %v, want sorted [a b c]", got.Select)
	}
}

func TestBuildPlan_DoNotGroupAcrossAgents(t *testing.T) {
	cands := []Candidate{
		{Kind: CandidateSkill, AgentName: "claude-code", SkillName: "a", SkillSource: "anthropics/skills"},
		{Kind: CandidateSkill, AgentName: "cursor", SkillName: "a", SkillSource: "anthropics/skills"},
	}
	plan := BuildPlan(cands, ScopeProject)
	if len(plan.Skills) != 2 {
		t.Fatalf("expected 2 entries (per agent), got %d", len(plan.Skills))
	}
}

func TestBuildPlan_DoNotGroupAcrossSources(t *testing.T) {
	cands := []Candidate{
		{Kind: CandidateSkill, AgentName: "claude-code", SkillName: "a", SkillSource: "anthropics/skills"},
		{Kind: CandidateSkill, AgentName: "claude-code", SkillName: "b", SkillSource: "vercel-labs/agent-skills"},
	}
	plan := BuildPlan(cands, ScopeProject)
	if len(plan.Skills) != 2 {
		t.Fatalf("expected 2 entries (per source), got %d", len(plan.Skills))
	}
}

func TestBuildPlan_MCPsNotGrouped(t *testing.T) {
	inline := &config.ConfigMcpItem{Command: "uvx", Args: []string{"mcp-server-git"}}
	cands := []Candidate{
		{Kind: CandidateMCP, AgentName: "claude-code", MCPName: "a", MCPTarget: "~/.claude.json", MCPInline: inline},
		{Kind: CandidateMCP, AgentName: "claude-code", MCPName: "b", MCPTarget: "~/.claude.json", MCPInline: inline},
	}
	plan := BuildPlan(cands, ScopeProject)
	if len(plan.MCPs) != 2 {
		t.Fatalf("expected 2 mcp entries, got %d", len(plan.MCPs))
	}
	if plan.MCPs[0].Name != "a" || plan.MCPs[1].Name != "b" {
		t.Errorf("mcps not sorted by name: %v, %v", plan.MCPs[0].Name, plan.MCPs[1].Name)
	}
}

func TestBuildPlan_SkillsSortedForStableOutput(t *testing.T) {
	cands := []Candidate{
		{Kind: CandidateSkill, AgentName: "cursor", SkillName: "a", SkillSource: "vercel-labs/agent-skills"},
		{Kind: CandidateSkill, AgentName: "claude-code", SkillName: "a", SkillSource: "anthropics/skills"},
	}
	plan := BuildPlan(cands, ScopeProject)
	if plan.Skills[0].Agents[0] != "claude-code" {
		t.Errorf("expected claude-code first (sorted by agent), got %v", plan.Skills[0].Agents)
	}
}

func TestBuildPlan_MCPUsesAgentAndGlobal(t *testing.T) {
	inline := &config.ConfigMcpItem{Command: "uvx", Args: []string{"mcp-server-git"}}
	cand := Candidate{
		Kind:      CandidateMCP,
		AgentName: "claude-code",
		MCPName:   "git",
		MCPTarget: "~/.config/claude/mcp.json", // kept for display, not written to plan
		MCPInline: inline,
	}
	plan := BuildPlan([]Candidate{cand}, ScopeGlobal)
	if len(plan.MCPs) != 1 {
		t.Fatalf("expected 1 mcp, got %d", len(plan.MCPs))
	}
	mcp := plan.MCPs[0]
	if mcp.Name != "git" {
		t.Errorf("name: got %q, want %q", mcp.Name, "git")
	}
	if mcp.Target != "" {
		t.Errorf("Target should be empty in new plan format, got %q", mcp.Target)
	}
	if !reflect.DeepEqual(mcp.Agents, []string{"claude-code"}) {
		t.Errorf("Agents: got %v, want [claude-code]", mcp.Agents)
	}
	if !mcp.Global {
		t.Error("Global should be true")
	}
}

func TestBuildPlan_MCPGlobalFollowsScope(t *testing.T) {
	inline := &config.ConfigMcpItem{Command: "uvx", Args: []string{"mcp-server-git"}}
	cand := Candidate{
		Kind:      CandidateMCP,
		AgentName: "claude-code",
		MCPName:   "git",
		MCPInline: inline,
	}

	tests := []struct {
		scope      Scope
		wantGlobal bool
	}{
		{ScopeGlobal, true},
		{ScopeProject, false},
	}

	for _, tc := range tests {
		plan := BuildPlan([]Candidate{cand}, tc.scope)
		if len(plan.MCPs) != 1 {
			t.Fatalf("scope %v: expected 1 mcp, got %d", tc.scope, len(plan.MCPs))
		}
		if plan.MCPs[0].Global != tc.wantGlobal {
			t.Errorf("scope %v: Global = %v, want %v", tc.scope, plan.MCPs[0].Global, tc.wantGlobal)
		}
	}
}
