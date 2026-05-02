package ops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// findSection returns the AgentSection matching name, or nil.
func findSection(c Candidates, name string) *AgentSection {
	for i := range c.Sections {
		if c.Sections[i].AgentName == name {
			return &c.Sections[i]
		}
	}
	return nil
}

func TestBuildImportCandidates_Empty(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	cacheRoot := t.TempDir()

	c, err := BuildImportCandidates(context.Background(), ScopeProject, home, workDir, cacheRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Sections) != 0 {
		t.Errorf("expected no sections, got %d", len(c.Sections))
	}
}

func TestBuildImportCandidates_ProjectScope_KeepsProjectSkills(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	cacheRoot := t.TempDir()

	// Project-level skill for claude-code.
	makeSkill(t, workDir, ".claude/skills", "frontend", "desc")

	c, err := BuildImportCandidates(context.Background(), ScopeProject, home, workDir, cacheRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sec := findSection(c, "claude-code")
	if sec == nil {
		t.Fatalf("missing claude-code section: %+v", c)
	}
	if len(sec.Skills) != 1 || sec.Skills[0].SkillName != "frontend" {
		t.Errorf("expected one skill named frontend, got %+v", sec.Skills)
	}
	if sec.Skills[0].SkillSourceKind != SourceKindPath {
		t.Errorf("expected SourceKindPath, got %q", sec.Skills[0].SkillSourceKind)
	}
}

func TestBuildImportCandidates_ProjectScope_DropsGlobalSkills(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	cacheRoot := t.TempDir()

	// Global-level skill for claude-code (should be hidden in project scope).
	makeSkill(t, home, ".claude/skills", "global-one", "")

	c, err := BuildImportCandidates(context.Background(), ScopeProject, home, workDir, cacheRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Sections) != 0 {
		t.Errorf("expected no sections, got: %+v", c.Sections)
	}
}

func TestBuildImportCandidates_GlobalScope_KeepsGlobalSkills(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	cacheRoot := t.TempDir()

	makeSkill(t, home, ".claude/skills", "global-one", "")

	c, err := BuildImportCandidates(context.Background(), ScopeGlobal, home, workDir, cacheRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sec := findSection(c, "claude-code")
	if sec == nil {
		t.Fatalf("missing claude-code section: %+v", c)
	}
	if len(sec.Skills) != 1 || sec.Skills[0].SkillName != "global-one" {
		t.Errorf("expected one global skill, got %+v", sec.Skills)
	}
}

func TestBuildImportCandidates_GlobalScope_DropsProjectSkills(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	cacheRoot := t.TempDir()

	makeSkill(t, workDir, ".claude/skills", "project-one", "")

	c, err := BuildImportCandidates(context.Background(), ScopeGlobal, home, workDir, cacheRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec := findSection(c, "claude-code"); sec != nil {
		t.Errorf("claude-code project skill must not appear in global scope: %+v", sec)
	}
}

func TestBuildImportCandidates_GenericSectionWithDelegators(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	cacheRoot := t.TempDir()

	makeSkill(t, workDir, ".agents/skills", "shared", "desc")

	c, err := BuildImportCandidates(context.Background(), ScopeProject, home, workDir, cacheRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sec := findSection(c, "generic")
	if sec == nil {
		t.Fatalf("missing generic section: %+v", c)
	}
	if len(sec.GenericDelegators) == 0 {
		t.Error("expected GenericDelegators to be populated in project scope")
	}
	if len(sec.Skills) != 1 || sec.Skills[0].SkillName != "shared" {
		t.Errorf("expected one shared skill, got %+v", sec.Skills)
	}
}

func TestBuildImportCandidates_MCPsFromAgentConfig(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	cacheRoot := t.TempDir()

	// claude-code reads its user-global MCP servers from ~/.claude.json.
	content := `{"mcpServers":{"filesystem":{"command":"uvx","args":["mcp-server-filesystem","/tmp"]}}}`
	cfgFile := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := BuildImportCandidates(context.Background(), ScopeProject, home, workDir, cacheRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sec := findSection(c, "claude-code")
	if sec == nil {
		t.Fatalf("missing claude-code section: %+v", c)
	}
	if len(sec.MCPs) != 1 || sec.MCPs[0].MCPName != "filesystem" {
		t.Errorf("expected filesystem mcp, got %+v", sec.MCPs)
	}
	if sec.MCPs[0].MCPInline == nil || sec.MCPs[0].MCPInline.Command != "uvx" {
		t.Errorf("expected inline loaded, got %+v", sec.MCPs[0].MCPInline)
	}
}

func TestBuildImportCandidates_SectionsSortedByAgent(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	cacheRoot := t.TempDir()

	// Drop one skill in a claude-code dir and one in generic.
	makeSkill(t, workDir, ".claude/skills", "one", "")
	makeSkill(t, workDir, ".agents/skills", "two", "")

	c, err := BuildImportCandidates(context.Background(), ScopeProject, home, workDir, cacheRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Sections) < 2 {
		t.Fatalf("expected at least 2 sections, got %+v", c.Sections)
	}
	for i := 1; i < len(c.Sections); i++ {
		if c.Sections[i-1].AgentName > c.Sections[i].AgentName {
			t.Errorf("sections not sorted: %q came before %q",
				c.Sections[i-1].AgentName, c.Sections[i].AgentName)
		}
	}
}
