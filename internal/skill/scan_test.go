package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// mkSkill creates a skill directory with a SKILL.md inside tmp/name/.
func mkSkill(t *testing.T, parent, name, content string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// --- ParseSkillMeta ---

func TestParseSkillMetaExported_WithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: my-skill\ndescription: Does something cool\n---\n\nBody text."
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	name, desc, err := ParseSkillMeta(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-skill" {
		t.Errorf("name: got %q, want %q", name, "my-skill")
	}
	if desc != "Does something cool" {
		t.Errorf("desc: got %q, want %q", desc, "Does something cool")
	}
}

func TestParseSkillMetaExported_NoName_FallbackToDirName(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "awesome-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\ndescription: no name here\n---"), 0o644); err != nil {
		t.Fatal(err)
	}

	name, desc, err := ParseSkillMeta(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "awesome-skill" {
		t.Errorf("name: got %q, want %q (dir fallback)", name, "awesome-skill")
	}
	if desc != "no name here" {
		t.Errorf("desc: got %q, want %q", desc, "no name here")
	}
}

func TestParseSkillMetaExported_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "plain")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte("Just plain body, no frontmatter."), 0o644); err != nil {
		t.Fatal(err)
	}

	name, _, err := ParseSkillMeta(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "plain" {
		t.Errorf("name: got %q, want dir fallback %q", name, "plain")
	}
}

func TestParseSkillMetaExported_NotFound(t *testing.T) {
	_, _, err := ParseSkillMeta("/no/such/path/SKILL.md")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// --- ScanDir ---

func TestScanDir_Empty(t *testing.T) {
	dir := t.TempDir()
	metas, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("expected 0 skills, got %d", len(metas))
	}
}

func TestScanDir_SubdirsWithSkillMD(t *testing.T) {
	dir := t.TempDir()
	mkSkill(t, dir, "alpha", "---\nname: alpha\ndescription: first\n---")
	mkSkill(t, dir, "beta", "---\nname: beta\ndescription: second\n---")
	// A regular file (not a dir) — should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	metas, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(metas))
	}
	names := map[string]bool{metas[0].Name: true, metas[1].Name: true}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("unexpected skill names: %v", names)
	}
}

func TestScanDir_RootIsASkill(t *testing.T) {
	dir := t.TempDir()
	// dir itself has a SKILL.md.
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: root-skill\n---"), 0o644); err != nil {
		t.Fatal(err)
	}

	metas, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(metas))
	}
	if metas[0].Name != "root-skill" {
		t.Errorf("expected root-skill, got %q", metas[0].Name)
	}
}

func TestScanDir_NonexistentDir_ReturnsEmpty(t *testing.T) {
	metas, err := ScanDir("/no/such/directory/at/all")
	if err != nil {
		t.Errorf("expected nil error for missing dir, got %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("expected 0 skills for missing dir, got %d", len(metas))
	}
}

func TestScanDir_NoRecursion(t *testing.T) {
	// ScanDir must NOT descend into sub-sub-dirs.
	dir := t.TempDir()
	nested := filepath.Join(dir, "outer", "inner")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("---\nname: deep\n---"), 0o644); err != nil {
		t.Fatal(err)
	}

	metas, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Neither outer nor inner should appear (outer has no SKILL.md, inner is too deep).
	if len(metas) != 0 {
		t.Errorf("expected 0 skills (no recursion), got %d: %+v", len(metas), metas)
	}
}

// --- WalkForSkillDirs ---

func TestWalkForSkillDirs_FindsSkillsDirs(t *testing.T) {
	root := t.TempDir()
	// root/ext-a/skills/  ← should be found
	// root/ext-a/skills/my-skill/SKILL.md
	extA := filepath.Join(root, "ext-a")
	skillsA := filepath.Join(extA, "skills")
	mkSkill(t, skillsA, "my-skill", "---\nname: my-skill\n---")

	// root/ext-b/other-dir/  ← should NOT be found (not named "skills")
	other := filepath.Join(root, "ext-b", "other-dir")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}

	dirs, err := WalkForSkillDirs(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("expected 1 skills dir, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != skillsA {
		t.Errorf("expected %q, got %q", skillsA, dirs[0])
	}
}

func TestWalkForSkillDirs_NoDescendIntoSkillsSubdirs(t *testing.T) {
	root := t.TempDir()
	// root/pkg/skills/sub/skills/  ← inner "skills" must NOT appear
	inner := filepath.Join(root, "pkg", "skills", "sub", "skills")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}

	dirs, err := WalkForSkillDirs(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only root/pkg/skills should appear, not the nested one.
	if len(dirs) != 1 {
		t.Fatalf("expected 1 skills dir, got %d: %v", len(dirs), dirs)
	}
	if filepath.Base(filepath.Dir(dirs[0])) != "pkg" {
		t.Errorf("expected root/pkg/skills, got %q", dirs[0])
	}
}

func TestWalkForSkillDirs_NonexistentRoot_ReturnsEmpty(t *testing.T) {
	dirs, err := WalkForSkillDirs("/no/such/path/at/all")
	if err != nil {
		t.Errorf("expected nil error for missing root, got %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs, got %d", len(dirs))
	}
}
