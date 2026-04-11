package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gaal/internal/config"
)

// TestInit_TemplateHasAllSections verifies the embedded template contains
// all required top-level YAML keys.
func TestInit_TemplateHasAllSections(t *testing.T) {
	for _, section := range []string{"repositories:", "skills:", "mcps:"} {
		if !strings.Contains(string(InitTemplate), section) {
			t.Errorf("InitTemplate missing section %q", section)
		}
	}
}

func TestInitFromPlan_WritesValidYAMLWithPlanEntries(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "gaal.yaml")
	plan := Plan{
		Skills: []config.SkillConfig{
			{
				Source: "anthropics/skills",
				Agents: []string{"claude-code"},
				Global: false,
				Select: []string{"frontend-design", "skill-creator"},
			},
		},
		MCPs: []config.MCPConfig{
			{
				Name:   "filesystem",
				Target: "~/.claude/mcp.json",
				Inline: &config.MCPInlineConfig{
					Command: "uvx",
					Args:    []string{"mcp-server-filesystem", "/tmp"},
				},
			},
		},
	}

	if err := InitFromPlan(dest, plan, false); err != nil {
		t.Fatalf("InitFromPlan: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"repositories:",
		"skills:",
		"mcps:",
		"anthropics/skills",
		"frontend-design",
		"skill-creator",
		"claude-code",
		"filesystem",
		"~/.claude/mcp.json",
		"uvx",
		"mcp-server-filesystem",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("output missing %q\n---\n%s\n---", want, content)
		}
	}

	cfg, err := config.Load(dest)
	if err != nil {
		t.Fatalf("generated file does not parse as valid config: %v\ncontent:\n%s", err, content)
	}
	if len(cfg.Skills) != 1 || cfg.Skills[0].Source != "anthropics/skills" {
		t.Errorf("skills: %+v", cfg.Skills)
	}
	if len(cfg.MCPs) != 1 || cfg.MCPs[0].Name != "filesystem" {
		t.Errorf("mcps: %+v", cfg.MCPs)
	}
}

func TestInitFromPlan_EmptyPlanStillValid(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "gaal.yaml")
	if err := InitFromPlan(dest, Plan{}, false); err != nil {
		t.Fatalf("InitFromPlan: %v", err)
	}
	if _, err := config.Load(dest); err != nil {
		t.Fatalf("empty plan output does not parse: %v", err)
	}
}

func TestInitFromPlan_PreservesCommentHeaders(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "gaal.yaml")
	if err := InitFromPlan(dest, Plan{}, false); err != nil {
		t.Fatalf("InitFromPlan: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	// A few characteristic comment markers from init_template.yaml.
	for _, want := range []string{
		"# gaal.yaml",
		"# repositories",
		"# skills",
		"# mcps",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("output missing header %q", want)
		}
	}
}

func TestInitFromPlan_ErrorWhenDestExistsWithoutForce(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "gaal.yaml")
	if err := os.WriteFile(dest, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InitFromPlan(dest, Plan{}, false); err == nil {
		t.Error("expected error when dest exists without force")
	}
}

func TestInitFromPlan_OverwritesWithForce(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "gaal.yaml")
	if err := os.WriteFile(dest, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InitFromPlan(dest, Plan{}, true); err != nil {
		t.Fatalf("InitFromPlan with force: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) == "existing" {
		t.Error("file was not overwritten")
	}
}
