package ops

import (
	"strings"
	"testing"
)

// TestOrDefault verifies the orDefault helper.
func TestOrDefault(t *testing.T) {
	if got := orDefault("", "fallback"); got != "fallback" {
		t.Errorf("orDefault empty: got %q, want fallback", got)
	}
	if got := orDefault("value", "fallback"); got != "value" {
		t.Errorf("orDefault non-empty: got %q, want value", got)
	}
}

// TestNonNil verifies the nonNil helper.
func TestNonNil(t *testing.T) {
	if got := nonNil(nil); got == nil {
		t.Error("nonNil(nil) returned nil, want empty slice")
	}
	if got := nonNil([]string{"a"}); len(got) != 1 || got[0] != "a" {
		t.Errorf("nonNil non-nil: unexpected %v", got)
	}
}

func TestCollectAgents_ResolvesGenericDirs(t *testing.T) {
	entries, err := collectAgents()
	if err != nil {
		t.Fatalf("collectAgents: %v", err)
	}

	for _, entry := range entries {
		if entry.Name != "cline" {
			continue
		}
		if entry.ProjectSkillsDir != ".agents/skills" {
			t.Errorf("cline ProjectSkillsDir = %q, want .agents/skills", entry.ProjectSkillsDir)
		}
		if entry.GlobalSkillsDir == "" {
			t.Fatal("cline GlobalSkillsDir is empty")
		}
		if strings.HasPrefix(entry.GlobalSkillsDir, "~") {
			t.Errorf("cline GlobalSkillsDir should be expanded, got %q", entry.GlobalSkillsDir)
		}
		if !entry.ProjectSkillsViaGeneric {
			t.Error("cline ProjectSkillsViaGeneric = false, want true")
		}
		if !entry.GlobalSkillsViaGeneric {
			t.Error("cline GlobalSkillsViaGeneric = false, want true")
		}
		return
	}

	t.Fatal("cline agent not found")
}
