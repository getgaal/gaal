package template

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"gaal/internal/config"
)

func TestGenerate_ContainsAllSections(t *testing.T) {
	out, err := Generate(config.ScopeWorkspace)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	for _, want := range []string{"schema: 1", "repositories:", "skills:", "mcps:"} {
		if !strings.Contains(s, want) {
			t.Errorf("template missing %q", want)
		}
	}
}

func TestGenerate_RepoEnumsPresent(t *testing.T) {
	out, err := Generate(config.ScopeWorkspace)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	for _, enum := range []string{"git", "hg", "svn", "bzr", "tar", "zip"} {
		if !strings.Contains(s, enum) {
			t.Errorf("template missing repo enum %q", enum)
		}
	}
}

func TestGenerate_TargetNotDocumented(t *testing.T) {
	out, err := Generate(config.ScopeWorkspace)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// "target" should not appear in the mcps Fields: block (deprecated).
	// We check it does not appear as a documented field name.
	s := string(out)
	// The mcps section starts at "mcps:"
	mcpsIdx := strings.Index(s, "# mcps\n")
	if mcpsIdx < 0 {
		t.Fatal("mcps section not found")
	}
	mcpsSection := s[mcpsIdx:]
	// target: should not be in the Fields comment lines
	for _, line := range strings.Split(mcpsSection, "\n") {
		if strings.HasPrefix(line, "#   target ") {
			t.Errorf("deprecated 'target' field should not appear in mcps Fields block, but got: %q", line)
		}
	}
}

func TestGenerate_AgentsDocumentedInMCPs(t *testing.T) {
	out, err := Generate(config.ScopeWorkspace)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	mcpsIdx := strings.Index(s, "# mcps\n")
	if mcpsIdx < 0 {
		t.Fatal("mcps section not found")
	}
	mcpsSection := s[mcpsIdx:]
	if !strings.Contains(mcpsSection, "agents") {
		t.Error("mcps section should document 'agents' field")
	}
}

func TestGenerate_InlineSubFieldsDocumented(t *testing.T) {
	out, err := Generate(config.ScopeWorkspace)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	// command (sub-field of inline) should be in the mcps section
	if !strings.Contains(s, "command") {
		t.Error("template should document inline.command sub-field")
	}
}

func TestGenerate_ContainsIntroAndCLICommands(t *testing.T) {
	out, err := Generate(config.ScopeWorkspace)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	for _, want := range []string{"gaal sync", "gaal status", "gaal audit"} {
		if !strings.Contains(s, want) {
			t.Errorf("template missing CLI hint %q", want)
		}
	}
}

func TestGenerate_SectionOrder(t *testing.T) {
	out, err := Generate(config.ScopeWorkspace)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	reposIdx := strings.Index(s, "repositories:")
	skillsIdx := strings.Index(s, "skills:")
	mcpsIdx := strings.Index(s, "mcps:")
	if reposIdx < 0 || skillsIdx < 0 || mcpsIdx < 0 {
		t.Fatal("one or more sections missing")
	}
	if !(reposIdx < skillsIdx && skillsIdx < mcpsIdx) {
		t.Errorf("sections out of order: repos=%d skills=%d mcps=%d", reposIdx, skillsIdx, mcpsIdx)
	}
}

func TestGenerate_IsValidYAML(t *testing.T) {
	out, err := Generate(config.ScopeWorkspace)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Must parse as a valid YAML document.
	var node yaml.Node
	if err := yaml.Unmarshal(out, &node); err != nil {
		t.Fatalf("generated template is not valid YAML: %v", err)
	}
	// Must unmarshal into a config.Config without error.
	var cfg config.Config
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatalf("generated template does not unmarshal into config.Config: %v", err)
	}
}

func TestGenerate_ScopeLabel(t *testing.T) {
	tests := []struct {
		scope config.ConfigScope
		want  string
	}{
		{config.ScopeWorkspace, "# Scope level: Workspace"},
		{config.ScopeUser, "# Scope level: User"},
		{config.ScopeGlobal, "# Scope level: Global"},
	}
	for _, tc := range tests {
		out, err := Generate(tc.scope)
		if err != nil {
			t.Fatalf("Generate(%v): %v", tc.scope, err)
		}
		if !strings.Contains(string(out), tc.want) {
			t.Errorf("scope %v: expected header to contain %q", tc.scope, tc.want)
		}
	}
}
