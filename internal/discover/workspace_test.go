package discover

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDepthOf validates the path depth helper.
func TestDepthOf(t *testing.T) {
	cases := []struct {
		rel  string
		want int
	}{
		{".", 0},
		{"a", 1},
		{filepath.Join("a", "b"), 2},
		{filepath.Join("a", "b", "c"), 3},
	}
	for _, tc := range cases {
		if got := depthOf(tc.rel); got != tc.want {
			t.Errorf("depthOf(%q) = %d, want %d", tc.rel, got, tc.want)
		}
	}
}

// TestScanWorkspace_empty returns nothing for an empty directory.
func TestScanWorkspace_empty(t *testing.T) {
	dir := t.TempDir()
	resources, err := scanWorkspace(t.Context(), dir, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resources))
	}
}

// TestScanWorkspace_withSkill finds a skill directory.
func TestScanWorkspace_withSkill(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "tools", "my-skill")
	makeSkillDir(t, skillDir, "my-skill", "")

	resources, err := scanWorkspace(t.Context(), root, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range resources {
		if r.Path == skillDir && r.Type == ResourceSkill {
			found = true
		}
	}
	if !found {
		t.Errorf("skill dir %q not found in results: %v", skillDir, resources)
	}
}

// TestScanWorkspace_withRepo detects a subdirectory with a .git marker as a repo.
func TestScanWorkspace_withRepo(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "clones", "my-repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	resources, err := scanWorkspace(t.Context(), root, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range resources {
		if r.Path == repoDir && r.Type == ResourceRepo {
			found = true
		}
	}
	if !found {
		t.Errorf("repo dir %q not found in results: %v", repoDir, resources)
	}
}

// TestScanWorkspace_rootNotRepo ensures the workDir root is not reported as a repo.
func TestScanWorkspace_rootNotRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	resources, err := scanWorkspace(t.Context(), root, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resources {
		if r.Path == root && r.Type == ResourceRepo {
			t.Errorf("workspace root should not be reported as a repo")
		}
	}
}

// TestScanWorkspace_maxDepth skips dirs beyond the depth limit.
func TestScanWorkspace_maxDepth(t *testing.T) {
	root := t.TempDir()
	// Place a skill at depth 5 (beyond default 4).
	deep := filepath.Join(root, "a", "b", "c", "d", "e", "deep-skill")
	makeSkillDir(t, deep, "deep-skill", "")

	resources, err := scanWorkspace(t.Context(), root, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resources {
		if r.Path == deep {
			t.Errorf("depth-5 skill should not appear with maxDepth=4")
		}
	}
}

// TestScanWorkspace_skipDirs does not descend into node_modules.
func TestScanWorkspace_skipDirs(t *testing.T) {
	root := t.TempDir()
	nm := filepath.Join(root, "node_modules", "some-skill")
	makeSkillDir(t, nm, "some-skill", "")

	resources, err := scanWorkspace(t.Context(), root, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resources {
		if r.Path == nm {
			t.Errorf("skill inside node_modules should be skipped")
		}
	}
}

// TestScanWorkspace_dedup does not report the same directory twice.
func TestScanWorkspace_dedup(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "tool")
	makeSkillDir(t, skillDir, "tool", "")

	resources, err := scanWorkspace(t.Context(), root, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, r := range resources {
		if r.Path == skillDir {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence, got %d", count)
	}
}

// TestComputeRepoDrift_unknown returns DriftUnknown for empty vcsType.
func TestComputeRepoDrift_unknown(t *testing.T) {
	got := computeRepoDrift(t.Context(), t.TempDir(), "")
	if got != DriftUnknown {
		t.Errorf("got %q, want DriftUnknown", got)
	}
}

// TestComputeRepoDrift_unknownVCSType returns DriftUnknown for unrecognised type.
func TestComputeRepoDrift_unknownVCSType(t *testing.T) {
	got := computeRepoDrift(t.Context(), t.TempDir(), "invalid-vcs-type")
	if got != DriftUnknown {
		t.Errorf("got %q, want DriftUnknown", got)
	}
}
