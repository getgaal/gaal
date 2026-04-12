package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"gaal/internal/skill"
)

func TestIsAgentInstalled_ProjectScope(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	// Create the parent dir of claude-code's project skills dir (.claude/).
	os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755)

	if !skill.IsAgentInstalled("claude-code", false, home, workDir) {
		t.Error("expected claude-code to be detected as installed (project scope)")
	}
}

func TestIsAgentInstalled_GlobalScope(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	// Create the parent dir of claude-code's global skills dir (~/.claude/).
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	if !skill.IsAgentInstalled("claude-code", true, home, workDir) {
		t.Error("expected claude-code to be detected as installed (global scope)")
	}
}

func TestIsAgentInstalled_Neither(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	// Don't create any dirs — should return false.
	if skill.IsAgentInstalled("claude-code", false, home, workDir) {
		t.Error("expected claude-code to NOT be detected (project scope)")
	}
	if skill.IsAgentInstalled("claude-code", true, home, workDir) {
		t.Error("expected claude-code to NOT be detected (global scope)")
	}
}

func TestIsAgentInstalled_UnknownAgent(t *testing.T) {
	if skill.IsAgentInstalled("nonexistent-agent", false, t.TempDir(), t.TempDir()) {
		t.Error("expected unknown agent to not be installed")
	}
}
