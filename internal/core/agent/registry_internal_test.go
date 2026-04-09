package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// ── loadInto ──────────────────────────────────────────────────────────────────

func TestLoadInto_ValidYAML(t *testing.T) {
	dst := map[string]Info{}
	data := []byte(`
agents:
  my-agent:
    project_skills_dir: .my/skills
    global_skills_dir: ~/.my/skills
    mcp_config_file: ~/.my/mcp.json
`)
	if err := loadInto(data, dst, false); err != nil {
		t.Fatalf("loadInto valid YAML: %v", err)
	}
	info, ok := dst["my-agent"]
	if !ok {
		t.Fatal("expected my-agent to be loaded")
	}
	if info.ProjectSkillsDir != ".my/skills" {
		t.Errorf("ProjectSkillsDir: got %q, want .my/skills", info.ProjectSkillsDir)
	}
	if info.GlobalSkillsDir != "~/.my/skills" {
		t.Errorf("GlobalSkillsDir: got %q, want ~/.my/skills", info.GlobalSkillsDir)
	}
	if info.MCPConfigFile != "~/.my/mcp.json" {
		t.Errorf("MCPConfigFile: got %q, want ~/.my/mcp.json", info.MCPConfigFile)
	}
}

func TestLoadInto_EmptyMCPConfigFile_IsAllowed(t *testing.T) {
	dst := map[string]Info{}
	data := []byte(`
agents:
  no-mcp:
    project_skills_dir: .agents/skills
    global_skills_dir: ~/.agents/skills
    mcp_config_file: ""
`)
	if err := loadInto(data, dst, false); err != nil {
		t.Fatalf("loadInto empty mcp_config_file: %v", err)
	}
	info := dst["no-mcp"]
	if info.MCPConfigFile != "" {
		t.Errorf("expected empty MCPConfigFile, got %q", info.MCPConfigFile)
	}
}

func TestLoadInto_InvalidYAML_ReturnsError(t *testing.T) {
	dst := map[string]Info{}
	if err := loadInto([]byte(":\t:bad yaml"), dst, false); err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadInto_Duplicate_NoOverride_ReturnsError(t *testing.T) {
	dst := map[string]Info{
		"existing": {ProjectSkillsDir: ".x/skills", GlobalSkillsDir: "~/.x/skills"},
	}
	data := []byte(`
agents:
  existing:
    project_skills_dir: .other/skills
    global_skills_dir: ~/.other/skills
    mcp_config_file: ""
`)
	if err := loadInto(data, dst, false); err == nil {
		t.Error("expected error for duplicate agent without allowOverride")
	}
}

func TestLoadInto_Duplicate_AllowOverride_Succeeds(t *testing.T) {
	dst := map[string]Info{
		"existing": {ProjectSkillsDir: ".x/skills", GlobalSkillsDir: "~/.x/skills"},
	}
	data := []byte(`
agents:
  existing:
    project_skills_dir: .new/skills
    global_skills_dir: ~/.new/skills
    mcp_config_file: ""
`)
	if err := loadInto(data, dst, true); err != nil {
		t.Fatalf("loadInto with allowOverride: %v", err)
	}
	if dst["existing"].ProjectSkillsDir != ".new/skills" {
		t.Errorf("expected override, got %q", dst["existing"].ProjectSkillsDir)
	}
}

// ── validateEntry ─────────────────────────────────────────────────────────────

func TestValidateEntry_Valid(t *testing.T) {
	e := agentEntry{
		ProjectSkillsDir: ".foo/skills",
		GlobalSkillsDir:  "~/.foo/skills",
		MCPConfigFile:    "~/.foo/mcp.json",
	}
	if err := validateEntry("foo", e); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateEntry_AbsoluteProjectDir_Rejected(t *testing.T) {
	e := agentEntry{
		ProjectSkillsDir: "/absolute/skills",
		GlobalSkillsDir:  "~/.foo/skills",
		MCPConfigFile:    "",
	}
	if err := validateEntry("foo", e); err == nil {
		t.Error("expected error for absolute project_skills_dir")
	}
}

func TestValidateEntry_DotDotInProjectDir_Rejected(t *testing.T) {
	e := agentEntry{
		ProjectSkillsDir: "../escape/skills",
		GlobalSkillsDir:  "~/.foo/skills",
		MCPConfigFile:    "",
	}
	if err := validateEntry("foo", e); err == nil {
		t.Error("expected error for '..' in project_skills_dir")
	}
}

func TestValidateEntry_GlobalDirWithoutTilde_Rejected(t *testing.T) {
	e := agentEntry{
		ProjectSkillsDir: ".foo/skills",
		GlobalSkillsDir:  "/home/user/.foo/skills", // absolute, not ~/
		MCPConfigFile:    "",
	}
	if err := validateEntry("foo", e); err == nil {
		t.Error("expected error for global_skills_dir without ~/")
	}
}

func TestValidateEntry_MCPConfigWithoutTilde_Rejected(t *testing.T) {
	e := agentEntry{
		ProjectSkillsDir: ".foo/skills",
		GlobalSkillsDir:  "~/.foo/skills",
		MCPConfigFile:    "/absolute/mcp.json",
	}
	if err := validateEntry("foo", e); err == nil {
		t.Error("expected error for mcp_config_file without ~/")
	}
}

// ── containsDotDot ────────────────────────────────────────────────────────────

func TestContainsDotDot(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"foo/bar", false},
		{"../escape", true},
		{"foo/../bar", true},
		{".dotfile/skills", false},
		{"..hidden", false}, // "..hidden" is not ".."
		{"..", true},
		{"foo/../../bar", true},
	}
	for _, tc := range cases {
		if got := containsDotDot(tc.input); got != tc.want {
			t.Errorf("containsDotDot(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// ── user agents file ──────────────────────────────────────────────────────────

func TestLoadInto_UserFile_AddsCustomAgent(t *testing.T) {
	dir := t.TempDir()
	userFile := filepath.Join(dir, "agents.yaml")
	content := []byte(`
agents:
  my-custom-agent:
    project_skills_dir: .custom/skills
    global_skills_dir: ~/.custom/skills
    mcp_config_file: ~/.custom/mcp.json
`)
	if err := os.WriteFile(userFile, content, 0o600); err != nil {
		t.Fatal(err)
	}

	dst := map[string]Info{}
	data, err := os.ReadFile(userFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := loadInto(data, dst, true); err != nil {
		t.Fatalf("loadInto user file: %v", err)
	}
	if _, ok := dst["my-custom-agent"]; !ok {
		t.Error("expected my-custom-agent to be loaded from user file")
	}
}

func TestLoadInto_UserFile_CannotOverrideBuiltin(t *testing.T) {
	// Simulate: built-in agents already in dst, user tries to override without allowOverride.
	dst := map[string]Info{
		"claude-code": {ProjectSkillsDir: ".claude/skills", GlobalSkillsDir: "~/.claude/skills"},
	}
	data := []byte(`
agents:
  claude-code:
    project_skills_dir: .evil/skills
    global_skills_dir: ~/.evil/skills
    mcp_config_file: ""
`)
	if err := loadInto(data, dst, false); err == nil {
		t.Error("expected error when attempting to override built-in agent")
	}
}
