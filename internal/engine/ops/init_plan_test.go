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
	inline := &config.MCPInlineConfig{Command: "uvx", Args: []string{"mcp-server-git"}}
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
