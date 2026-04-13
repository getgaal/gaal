package discover

import (
	"os"
	"path/filepath"
	"testing"
)

// makeSkillDir creates a minimal skill directory with a SKILL.md at dir.
func makeSkillDir(t *testing.T, dir, name, desc string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestParseSkillFrontmatter_valid parses name and description.
func TestParseSkillFrontmatter_valid(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(md, []byte("---\nname: my-skill\ndescription: A test skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	name, desc, err := parseSkillFrontmatter(md)
	if err != nil {
		t.Fatal(err)
	}
	if name != "my-skill" {
		t.Errorf("name: got %q, want %q", name, "my-skill")
	}
	if desc != "A test skill" {
		t.Errorf("desc: got %q, want %q", desc, "A test skill")
	}
}

// TestParseSkillFrontmatter_noFrontmatter returns empty strings gracefully.
func TestParseSkillFrontmatter_noFrontmatter(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(md, []byte("# Just a skill, no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	name, _, err := parseSkillFrontmatter(md)
	if err != nil {
		t.Fatal(err)
	}
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
}

// TestParseSkillFrontmatter_missing returns an error.
func TestParseSkillFrontmatter_missing(t *testing.T) {
	_, _, err := parseSkillFrontmatter(filepath.Join(t.TempDir(), "SKILL.md"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// TestSkillName_frontmatter uses frontmatter name when available.
func TestSkillName_frontmatter(t *testing.T) {
	dir := t.TempDir()
	makeSkillDir(t, dir, "from-frontmatter", "desc")
	if got := skillName(dir); got != "from-frontmatter" {
		t.Errorf("got %q, want %q", got, "from-frontmatter")
	}
}

// TestSkillName_fallback falls back to directory base name.
func TestSkillName_fallback(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-skill-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := skillName(dir); got != "my-skill-dir" {
		t.Errorf("got %q, want %q", got, "my-skill-dir")
	}
}

// TestHasVCSMarker detects a .git directory.
func TestHasVCSMarker(t *testing.T) {
	dir := t.TempDir()
	if hasVCSMarker(dir) {
		t.Error("expected false for dir without VCS marker")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hasVCSMarker(dir) {
		t.Error("expected true for dir with .git")
	}
}

// TestSkillsFromDir_basic finds a skill inside a parent directory.
func TestSkillsFromDir_basic(t *testing.T) {
	parent := t.TempDir()
	skillDir := filepath.Join(parent, "my-skill")
	makeSkillDir(t, skillDir, "my-skill", "")

	seen := make(map[string]struct{})
	results := skillsFromDir(t.Context(), parent, ScopeGlobal, "test-agent", "", seen)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Path != skillDir {
		t.Errorf("path: got %q, want %q", results[0].Path, skillDir)
	}
	if results[0].Meta["agent"] != "test-agent" {
		t.Errorf("agent: got %q", results[0].Meta["agent"])
	}
}

// TestSkillsFromDir_dedup does not return the same skill twice.
func TestSkillsFromDir_dedup(t *testing.T) {
	parent := t.TempDir()
	skillDir := filepath.Join(parent, "my-skill")
	makeSkillDir(t, skillDir, "my-skill", "")

	seen := make(map[string]struct{})
	r1 := skillsFromDir(t.Context(), parent, ScopeGlobal, "agent-a", "", seen)
	r2 := skillsFromDir(t.Context(), parent, ScopeGlobal, "agent-b", "", seen)
	if len(r1) != 1 || len(r2) != 0 {
		t.Errorf("dedup failed: r1=%d r2=%d", len(r1), len(r2))
	}
}

// TestSkillsFromDir_rootIsSkill detects when the dir itself is a skill root.
func TestSkillsFromDir_rootIsSkill(t *testing.T) {
	dir := t.TempDir()
	makeSkillDir(t, dir, "root-skill", "")

	seen := make(map[string]struct{})
	results := skillsFromDir(t.Context(), dir, ScopeGlobal, "agent", "", seen)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Path != dir {
		t.Errorf("expected root dir as skill path")
	}
}

// TestSkillsFromDir_emptyDir returns nothing for a non-existent directory.
func TestSkillsFromDir_emptyDir(t *testing.T) {
	seen := make(map[string]struct{})
	results := skillsFromDir(t.Context(), filepath.Join(t.TempDir(), "nope"), ScopeGlobal, "agent", "", seen)
	if len(results) != 0 {
		t.Errorf("expected 0 results for missing dir, got %d", len(results))
	}
}

// TestComputeSkillDrift_noSnapshot returns DriftUnknown without a stateDir.
func TestComputeSkillDrift_noSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: x\n---\n")
	got := computeSkillDrift(t.Context(), dir, "")
	if got != DriftUnknown {
		t.Errorf("got %q, want DriftUnknown", got)
	}
}

// TestComputeSkillDrift_ok returns DriftOK when snapshot matches disk.
func TestComputeSkillDrift_ok(t *testing.T) {
	stateDir := t.TempDir()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: x\n---\n")

	snap, _ := SnapshotDir(dir)
	key := "skill-" + WorkdirKey(dir)
	if err := Save(SnapshotPath(stateDir, key), snap); err != nil {
		t.Fatal(err)
	}
	got := computeSkillDrift(t.Context(), dir, stateDir)
	if got != DriftOK {
		t.Errorf("got %q, want DriftOK", got)
	}
}

// TestComputeSkillDrift_modified returns DriftModified when a file changed.
func TestComputeSkillDrift_modified(t *testing.T) {
	stateDir := t.TempDir()
	dir := t.TempDir()
	p := filepath.Join(dir, "SKILL.md")
	writeFile(t, p, "---\nname: x\n---\n")

	snap, _ := SnapshotDir(dir)
	key := "skill-" + WorkdirKey(dir)
	_ = Save(SnapshotPath(stateDir, key), snap)

	// Modify the file.
	if err := os.WriteFile(p, []byte("changed content"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := computeSkillDrift(t.Context(), dir, stateDir)
	if got != DriftModified {
		t.Errorf("got %q, want DriftModified", got)
	}
}

// TestWalkSkillDirs finds nested "skills" directories.
func TestWalkSkillDirs(t *testing.T) {
	root := t.TempDir()
	s1 := filepath.Join(root, "pkg", "skills")
	s2 := filepath.Join(root, "other", "skills")
	for _, d := range []string{s1, s2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dirs, err := walkSkillDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 {
		t.Errorf("expected 2, got %d: %v", len(dirs), dirs)
	}
}

// TestWalkSkillDirs_missing returns no error for a non-existent root.
func TestWalkSkillDirs_missing(t *testing.T) {
	dirs, err := walkSkillDirs(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs, got %d", len(dirs))
	}
}
