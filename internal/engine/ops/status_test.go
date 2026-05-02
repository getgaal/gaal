package ops

import (
	"path/filepath"
	"strings"
	"testing"

	"gaal/internal/discover"
	"gaal/internal/engine/render"
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

// TestIsUnderAny exercises the path-containment helper that powers the source
// suppression logic in reconcileSkills.
func TestIsUnderAny(t *testing.T) {
	roots := []string{"/home/u/src/personal-skills"}

	tests := []struct {
		path string
		want bool
	}{
		{"/home/u/src/personal-skills", true},                // root itself
		{"/home/u/src/personal-skills/crm-update", true},     // direct child
		{"/home/u/src/personal-skills/a/b/c/SKILL.md", true}, // deeper descendant
		{"/home/u/src/personal-skills-other", false},         // sibling with shared prefix
		{"/home/u/other", false},                             // unrelated
		{"/home/u/src", false},                               // ancestor of root
	}
	for _, tc := range tests {
		if got := isUnderAny(tc.path, roots); got != tc.want {
			t.Errorf("isUnderAny(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}

	// Empty roots / empty entries are always false.
	if isUnderAny("/anything", nil) {
		t.Error("isUnderAny with nil roots should be false")
	}
	if isUnderAny("/x", []string{""}) {
		t.Error("isUnderAny ignoring empty roots should be false")
	}
}

// TestReconcileSkills_SuppressesFSDiscoveryInsideConfiguredSource is the
// regression test for issue #88: when gaal is run from inside a configured
// skill source (e.g. the user's `~/.config/gaal/src/personal-skills` clone),
// scanWorkspace finds the source's SKILL.md files and emits them as
// agent-less workspace skills. They must not surface as duplicate rows.
func TestReconcileSkills_SuppressesFSDiscoveryInsideConfiguredSource(t *testing.T) {
	home := "/home/test"
	workDir := "/home/test/work"
	src := filepath.Join(home, ".config", "gaal", "src", "personal-skills")

	// Config-driven entry: claude-code installs crm-update globally.
	config := []render.SkillEntry{{
		Source:    src,
		Agent:     "claude-code",
		Global:    true,
		Status:    render.StatusOK,
		Installed: []string{"crm-update"},
		Missing:   []string{},
		Modified:  []string{},
	}}

	// Workspace scan finds the SKILL.md inside the source clone — no agent.
	resources := []discover.Resource{{
		Type:  discover.ResourceSkill,
		Scope: discover.ScopeWorkspace,
		Path:  filepath.Join(src, "crm-update"),
		Name:  "crm-update",
	}}

	got := reconcileSkills(config, resources, home, workDir, []string{src})

	if len(got) != 1 {
		t.Fatalf("expected exactly the config entry, got %d entries: %+v", len(got), got)
	}
	if got[0].Agent != "claude-code" {
		t.Errorf("expected the surviving entry to be the config one, got Agent=%q", got[0].Agent)
	}
}

// TestReconcileSkills_KeepsUnrelatedFSDiscovery makes sure the source-path
// filter only suppresses skills inside *configured* sources. A workspace
// skill found elsewhere is still surfaced as unmanaged.
func TestReconcileSkills_KeepsUnrelatedFSDiscovery(t *testing.T) {
	home := "/home/test"
	workDir := "/home/test/work"

	resources := []discover.Resource{{
		Type:  discover.ResourceSkill,
		Scope: discover.ScopeWorkspace,
		Path:  "/some/random/skill-dir",
		Name:  "stray",
	}}

	got := reconcileSkills(nil, resources, home, workDir, []string{"/configured/source"})

	if len(got) != 1 {
		t.Fatalf("expected 1 unmanaged entry, got %d: %+v", len(got), got)
	}
	if got[0].Status != render.StatusUnmanaged {
		t.Errorf("expected StatusUnmanaged, got %q", got[0].Status)
	}
}
