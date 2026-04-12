package ops

import (
	"os"
	"path/filepath"
	"testing"

	"gaal/internal/core/agent"
	"gaal/internal/engine/render"
)

func TestListAgents_AllHaveNames(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	entries, err := ListAgents(home, workDir)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one agent entry")
	}
	for _, e := range entries {
		if e.Name == "" {
			t.Error("agent entry has empty name")
		}
	}
}

func TestListAgents_InstalledDetection(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	// Create claude-code's global parent dir so it's detected as installed.
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	entries, err := ListAgents(home, workDir)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Name == "claude-code" {
			found = true
			if !e.Installed {
				t.Error("expected claude-code Installed=true")
			}
			break
		}
	}
	if !found {
		t.Error("claude-code not found in list")
	}
}

func TestListAgents_SortOrder(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	// Install only claude-code (global).
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	entries, err := ListAgents(home, workDir)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	// Find the boundary: installed entries should precede uninstalled.
	seenUninstalled := false
	for _, e := range entries {
		if !e.Installed {
			seenUninstalled = true
		} else if seenUninstalled {
			t.Errorf("installed agent %q appears after uninstalled agents", e.Name)
		}
	}
	// Within installed and uninstalled groups, names should be sorted.
	var prevInstalled, prevUninstalled string
	for _, e := range entries {
		if e.Installed {
			if e.Name < prevInstalled {
				t.Errorf("installed agents not sorted: %q after %q", e.Name, prevInstalled)
			}
			prevInstalled = e.Name
		} else {
			if e.Name < prevUninstalled {
				t.Errorf("uninstalled agents not sorted: %q after %q", e.Name, prevUninstalled)
			}
			prevUninstalled = e.Name
		}
	}
}

func TestListAgents_SourceField(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	entries, err := ListAgents(home, workDir)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	builtinSet := make(map[string]struct{}, len(agent.Names()))
	for _, n := range agent.Names() {
		builtinSet[n] = struct{}{}
	}
	for _, e := range entries {
		if _, ok := builtinSet[e.Name]; ok {
			if e.Source != "builtin" {
				t.Errorf("agent %q: expected source=builtin, got %q", e.Name, e.Source)
			}
		}
	}
}

func TestAgentDetail_Found(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	detail, err := AgentDetail(home, workDir, "claude-code")
	if err != nil {
		t.Fatalf("AgentDetail: %v", err)
	}
	if detail.Name != "claude-code" {
		t.Errorf("expected name=claude-code, got %q", detail.Name)
	}
	if detail.Source != "builtin" {
		t.Errorf("expected source=builtin, got %q", detail.Source)
	}
	if !detail.MCPSupport {
		t.Error("expected MCPSupport=true for claude-code")
	}
	if len(detail.Paths) == 0 {
		t.Error("expected at least one path entry")
	}
}

func TestAgentDetail_CaseInsensitive(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	detail, err := AgentDetail(home, workDir, "Claude-Code")
	if err != nil {
		t.Fatalf("AgentDetail: %v", err)
	}
	if detail.Name != "claude-code" {
		t.Errorf("expected canonical name claude-code, got %q", detail.Name)
	}
}

func TestAgentDetail_NotFound(t *testing.T) {
	_, err := AgentDetail(t.TempDir(), t.TempDir(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestAgentDetail_SkillCount(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()

	// Create a fake project skills dir with 2 SKILL.md files.
	skillsDir := filepath.Join(workDir, ".claude", "skills")
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(skillsDir, name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644)
	}

	detail, err := AgentDetail(home, workDir, "claude-code")
	if err != nil {
		t.Fatalf("AgentDetail: %v", err)
	}

	var projectPath *render.AgentPath
	for i := range detail.Paths {
		if detail.Paths[i].Label == "project" {
			projectPath = &detail.Paths[i]
			break
		}
	}
	if projectPath == nil {
		t.Fatal("no project path found in detail")
	}
	if !projectPath.Exists {
		t.Error("expected project path to exist")
	}
	if projectPath.SkillCount != 2 {
		t.Errorf("expected 2 skills, got %d", projectPath.SkillCount)
	}
}
