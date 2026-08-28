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

func TestImmediateVCSChildren_FindsOnlyDirectVCSChildren(t *testing.T) {
	root := t.TempDir()

	// Two real repo-like children (a plain .git dir is enough to trigger
	// hasVCSMarker — no real git binary or history needed).
	mustMkdirAll(t, filepath.Join(root, "frontend", ".git"))
	mustMkdirAll(t, filepath.Join(root, "api", ".git"))

	// A non-repo directory must be skipped.
	mustMkdirAll(t, filepath.Join(root, "docs"))

	// A VCS marker nested two levels down must NOT be treated as an
	// immediate child — this only scans the direct children of root.
	mustMkdirAll(t, filepath.Join(root, "docs", "nested-repo", ".git"))

	// A skipDirs entry (node_modules) must be skipped even if it somehow
	// contained a VCS marker.
	mustMkdirAll(t, filepath.Join(root, "node_modules", ".git"))

	got, err := ImmediateVCSChildren(root)
	if err != nil {
		t.Fatalf("ImmediateVCSChildren: %v", err)
	}
	want := []string{"api", "frontend"} // alphabetical
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q (full: %v)", i, got[i], w, got)
		}
	}
}

func TestImmediateVCSChildren_NoVCSChildren_ReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "not-a-repo"))

	got, err := ImmediateVCSChildren(root)
	if err != nil {
		t.Fatalf("ImmediateVCSChildren: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no VCS children, got %v", got)
	}
}

func TestImmediateVCSChildren_MissingWorkDir_ReturnsEmptyNoError(t *testing.T) {
	got, err := ImmediateVCSChildren(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected nil error for missing workDir, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil result, got %v", got)
	}
}

func TestImmediateVCSChildren_RecognisesEveryVCSMarker(t *testing.T) {
	root := t.TempDir()
	for _, marker := range []string{".git", ".hg", ".svn", ".bzr"} {
		mustMkdirAll(t, filepath.Join(root, marker+"-repo", marker))
	}

	got, err := ImmediateVCSChildren(root)
	if err != nil {
		t.Fatalf("ImmediateVCSChildren: %v", err)
	}
	if len(got) != 4 {
		t.Errorf("expected 4 VCS children (one per marker type), got %d: %v", len(got), got)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
