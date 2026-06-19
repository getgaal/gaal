package skill

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gaal/internal/config"
)

// buildSkillDir creates a directory with a SKILL.md file and returns the root.
func buildSkillDir(t *testing.T, name, description string) string {
	t.Helper()
	root := t.TempDir()
	skillDir := filepath.Join(root, name)
	os.MkdirAll(skillDir, 0o755)
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\n"
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
	return root
}

// ---------------------------------------------------------------------------
// parseSkillMeta
// ---------------------------------------------------------------------------

func TestParseSkillMeta_WithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "SKILL.md")
	os.WriteFile(p, []byte("---\nname: my-skill\ndescription: A test skill\n---\n# Body\n"), 0o644)

	meta, err := parseSkillMeta(p)
	if err != nil {
		t.Fatalf("parseSkillMeta: %v", err)
	}
	if meta.Name != "my-skill" {
		t.Errorf("got Name=%q, want my-skill", meta.Name)
	}
	if meta.Description != "A test skill" {
		t.Errorf("got Description=%q, want 'A test skill'", meta.Description)
	}
}

func TestParseSkillMeta_NoFrontmatter_FallsBackToDirName(t *testing.T) {
	// Place SKILL.md inside a named directory so the fallback picks up the name.
	dir := filepath.Join(t.TempDir(), "my-fallback-skill")
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "SKILL.md")
	os.WriteFile(p, []byte("# No frontmatter here\n"), 0o644)

	meta, err := parseSkillMeta(p)
	if err != nil {
		t.Fatalf("parseSkillMeta: %v", err)
	}
	if meta.Name != "my-fallback-skill" {
		t.Errorf("expected fallback name 'my-fallback-skill', got %q", meta.Name)
	}
}

func TestParseSkillMeta_MissingFile(t *testing.T) {
	_, err := parseSkillMeta("/no/such/SKILL.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// discoverSkills
// ---------------------------------------------------------------------------

func TestDiscoverSkills_RootContainsSkill(t *testing.T) {
	root := t.TempDir()
	// Place skill in a subdirectory of root — standard discovery location.
	skillDir := filepath.Join(root, "my-tool")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-tool\n---\n"), 0o644)

	skills, err := discoverSkills(root)
	if err != nil {
		t.Fatalf("discoverSkills: %v", err)
	}
	if len(skills) == 0 {
		t.Error("expected at least one skill")
	}
}

func TestDiscoverSkills_EmptyRoot(t *testing.T) {
	root := t.TempDir()
	skills, err := discoverSkills(root)
	if err != nil {
		t.Fatalf("discoverSkills: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills in empty dir, got %d", len(skills))
	}
}

func TestDiscoverSkills_NoDuplicates(t *testing.T) {
	// Same skill referenced via multiple discovery dirs should not be duplicated.
	root := t.TempDir()
	skillDir := filepath.Join(root, "cool-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: cool-skill\n---\n"), 0o644)

	skills, err := discoverSkills(root)
	if err != nil {
		t.Fatalf("discoverSkills: %v", err)
	}

	seen := map[string]int{}
	for _, sk := range skills {
		seen[sk.Dir]++
	}
	for dir, count := range seen {
		if count > 1 {
			t.Errorf("skill dir %q discovered %d times", dir, count)
		}
	}
}

// ---------------------------------------------------------------------------
// filterSkills
// ---------------------------------------------------------------------------

func TestFilterSkills_EmptySelect_ReturnsAll(t *testing.T) {
	all := []SkillMeta{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := filterSkills(all, nil)
	if len(got) != 3 {
		t.Errorf("expected 3, got %d", len(got))
	}
}

func TestFilterSkills_Select_Subset(t *testing.T) {
	all := []SkillMeta{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := filterSkills(all, []string{"a", "c"})
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
}

func TestFilterSkills_Select_DirectoryName(t *testing.T) {
	all := []SkillMeta{{Name: "firecrawl", Dir: filepath.Join(t.TempDir(), "firecrawl-cli")}}
	got := filterSkills(all, []string{"firecrawl-cli"})
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Name != "firecrawl" {
		t.Errorf("expected selected skill name firecrawl, got %q", got[0].Name)
	}
}

func TestFilterSkills_Select_NoMatch(t *testing.T) {
	all := []SkillMeta{{Name: "a"}, {Name: "b"}}
	got := filterSkills(all, []string{"z"})
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestSyncOne_WarnsWhenSelectMatchesNoSkill(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "firecrawl-cli")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: firecrawl\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	m := NewManager(nil, t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), false)
	if err := m.syncOne(context.Background(), config.ConfigSkill{Source: root, Select: []string{"missing"}}); err != nil {
		t.Fatalf("syncOne: %v", err)
	}

	log := buf.String()
	if !strings.Contains(log, "no selected skills found in source") {
		t.Fatalf("expected no-selected warning, got %q", log)
	}
	if strings.Contains(log, "msg=\"no skills found in source\"") {
		t.Fatalf("expected no generic no-skills warning, got %q", log)
	}
}

// ---------------------------------------------------------------------------
// installSkill
// ---------------------------------------------------------------------------

func TestInstallSkill_CopiesFiles(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# skill"), 0o644)
	os.WriteFile(filepath.Join(src, "helper.sh"), []byte("#!/bin/sh"), 0o755)

	dst := filepath.Join(t.TempDir(), "installed-skill")
	if err := installSkill(src, dst); err != nil {
		t.Fatalf("installSkill: %v", err)
	}

	for _, name := range []string{"SKILL.md", "helper.sh"} {
		if _, err := os.Stat(filepath.Join(dst, name)); err != nil {
			t.Errorf("expected file %q in dst: %v", name, err)
		}
	}
}

func TestInstallSkill_CreatesNestedDirs(t *testing.T) {
	src := t.TempDir()
	subDir := filepath.Join(src, "sub")
	os.MkdirAll(subDir, 0o755)
	os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("nested"), 0o644)

	dst := filepath.Join(t.TempDir(), "dest")
	if err := installSkill(src, dst); err != nil {
		t.Fatalf("installSkill nested: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "sub", "file.txt")); err != nil {
		t.Error("expected nested file to exist in dst")
	}
}

// TestInstallSkill_SkipsVCSMetadata exercises the fix for issue #86: when a
// skill source is a VCS clone, copying the .git/ tree to the destination
// burdens it with read-only pack files (mode 0o444) that the next sync cannot
// overwrite. installSkill must skip the VCS metadata directories entirely.
func TestInstallSkill_SkipsVCSMetadata(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# skill"), 0o644)
	for _, meta := range []string{".git", ".hg", ".svn", ".bzr"} {
		dir := filepath.Join(src, meta, "objects", "pack")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Mode 0o444 mimics go-git's read-only pack files. If installSkill
		// walked into the metadata dir the second sync would fail.
		if err := os.WriteFile(filepath.Join(dir, "pack-x.idx"), []byte("x"), 0o444); err != nil {
			t.Fatal(err)
		}
	}

	dst := filepath.Join(t.TempDir(), "installed")
	if err := installSkill(src, dst); err != nil {
		t.Fatalf("installSkill: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		t.Errorf("expected SKILL.md in dst: %v", err)
	}
	for _, meta := range []string{".git", ".hg", ".svn", ".bzr"} {
		if _, err := os.Stat(filepath.Join(dst, meta)); !os.IsNotExist(err) {
			t.Errorf("expected %s/ to be skipped, got err=%v", meta, err)
		}
	}
}

// TestInstallSkill_IsIdempotent ensures a second install over an existing
// destination does not fail. This guards the regression in issue #86: the
// previous implementation copied read-only pack files from .git/ on the
// first run, which the second run could not overwrite.
func TestInstallSkill_IsIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only enforcement on Windows differs from POSIX")
	}
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# skill"), 0o644)
	gitPack := filepath.Join(src, ".git", "objects", "pack")
	if err := os.MkdirAll(gitPack, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitPack, "pack-1.idx"), []byte("idx"), 0o444); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "installed")
	if err := installSkill(src, dst); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := installSkill(src, dst); err != nil {
		t.Fatalf("second install: %v", err)
	}
}

// TestInstallSkill_PreservesNestedNonVCSDotDirs ensures only the well-known
// VCS metadata dirs are skipped. A skill that ships its own dotdir (e.g. a
// `.config/` directory of templates) must still be copied through.
func TestInstallSkill_PreservesNestedNonVCSDotDirs(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# skill"), 0o644)
	cfgDir := filepath.Join(src, ".config")
	os.MkdirAll(cfgDir, 0o755)
	os.WriteFile(filepath.Join(cfgDir, "template.yml"), []byte("a: 1"), 0o644)

	dst := filepath.Join(t.TempDir(), "installed")
	if err := installSkill(src, dst); err != nil {
		t.Fatalf("installSkill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".config", "template.yml")); err != nil {
		t.Errorf("expected .config/template.yml to be copied, got err=%v", err)
	}
}

// ---------------------------------------------------------------------------
// Manager.detectInstalledAgents (project scope)
// ---------------------------------------------------------------------------

func TestDetectInstalledAgents_ProjectScope(t *testing.T) {
	workDir := t.TempDir()

	// Create the parent config dir for one known agent (claude-code uses .claude/).
	os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755)

	m := NewManager(nil, t.TempDir(), "/home/testuser", workDir, "", false)
	found := m.detectInstalledAgents(false)

	hadAgent := false
	for _, name := range found {
		if name == "claude-code" {
			hadAgent = true
		}
	}
	if !hadAgent {
		t.Errorf("expected claude-code to be detected when .claude/ exists; found: %v", found)
	}
}

// ---------------------------------------------------------------------------
// isLocalPath
// ---------------------------------------------------------------------------

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// POSIX paths
		{"/absolute/path", true},
		{"./relative", true},
		{"../parent", true},
		{"~/home", true},
		// Windows-style paths
		{`C:\Users\foo`, true},
		{`~\myskills`, true},
		{`.\rel`, true},
		{`..\parent`, true},
		// Not local
		{"owner/repo", false},
		{"https://github.com/foo/bar", false},
	}
	for _, tc := range tests {
		got := isLocalPath(tc.input)
		if got != tc.want {
			t.Errorf("isLocalPath(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Manager.SourcePaths
// ---------------------------------------------------------------------------

func TestManager_SourcePaths_ExpandsHomeAndKeepsAbsolute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("expectations hard-code POSIX absolute paths; needs filepath.Join/ToSlash port (tracked in #231)")
	}
	skills := []config.ConfigSkill{
		{Source: "~/skills/personal"},
		{Source: "/abs/skills"},
	}
	m := NewManager(skills, "/cache", "/home/u", "/work", "", false)

	got := m.SourcePaths()
	want := []string{"/home/u/skills/personal", "/abs/skills"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("SourcePaths[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestManager_SourcePaths_RemoteResolvesToCachePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HasPrefix \"/cache\" assertion uses POSIX separators; needs filepath.ToSlash port (tracked in #231)")
	}
	skills := []config.ConfigSkill{
		{Source: "owner/repo"},
		{Source: "https://github.com/owner/repo2"},
	}
	m := NewManager(skills, "/cache", "/home/u", "/work", "", false)

	got := m.SourcePaths()
	if len(got) != 2 {
		t.Fatalf("expected 2 paths, got %v", got)
	}
	for _, p := range got {
		if !filepath.IsAbs(p) {
			t.Errorf("expected absolute cache path, got %q", p)
		}
		if !strings.HasPrefix(p, "/cache") {
			t.Errorf("expected path under /cache, got %q", p)
		}
	}
}

// ---------------------------------------------------------------------------
// Manager.Status
// ---------------------------------------------------------------------------

func TestManager_Status_UnknownAgent(t *testing.T) {
	sourceDir := t.TempDir()
	skills := []config.ConfigSkill{
		{Source: sourceDir, Agents: []string{"unknown-agent-xyz"}},
	}
	m := NewManager(skills, t.TempDir(), "/home/user", t.TempDir(), "", false)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Err == nil {
		t.Error("expected error status for unknown agent")
	}
}

func TestManager_Status_SourceNotCached(t *testing.T) {
	// Non-local source that has never been downloaded.
	skills := []config.ConfigSkill{
		{Source: "owner/never-downloaded-repo", Agents: []string{"claude-code"}},
	}
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755)
	m := NewManager(skills, t.TempDir(), "/home/user", workDir, "", false)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Err == nil {
		t.Error("expected 'source not cached yet' error")
	}
}

func TestManager_Status_InstalledAndMissing(t *testing.T) {
	// Source with two skills.
	sourceDir := t.TempDir()
	for _, name := range []string{"skill-a", "skill-b"} {
		d := filepath.Join(sourceDir, name)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644)
	}

	// Simulate workDir with .claude skills dir, only skill-a installed.
	workDir := t.TempDir()
	claudeSkillsDir := filepath.Join(workDir, ".claude", "skills")
	os.MkdirAll(filepath.Join(claudeSkillsDir, "skill-a"), 0o755) // installed

	skills := []config.ConfigSkill{
		{Source: sourceDir, Agents: []string{"claude-code"}, Global: false},
	}
	m := NewManager(skills, t.TempDir(), "/home/user", workDir, "", false)
	statuses := m.Status(context.Background())

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status entry, got %d", len(statuses))
	}
	st := statuses[0]
	if st.Err != nil {
		t.Fatalf("unexpected error: %v", st.Err)
	}

	hasInstalled := false
	for _, n := range st.Installed {
		if n == "skill-a" {
			hasInstalled = true
		}
	}
	if !hasInstalled {
		t.Errorf("expected skill-a in Installed, got: %v", st.Installed)
	}

	hasMissing := false
	for _, n := range st.Missing {
		if n == "skill-b" {
			hasMissing = true
		}
	}
	if !hasMissing {
		t.Errorf("expected skill-b in Missing, got: %v", st.Missing)
	}
}

// ---------------------------------------------------------------------------
// Manager.syncOne with unknown agent (warn + skip branch)
// ---------------------------------------------------------------------------

func TestManager_Sync_UnknownAgent_SkipsWithWarn(t *testing.T) {
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644)

	skills := []config.ConfigSkill{
		{Source: sourceDir, Agents: []string{"unknown-agent-xyz"}},
	}
	m := NewManager(skills, t.TempDir(), "/home/user", t.TempDir(), "", false)
	// Should not error — unknown agent is warned and skipped.
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync with unknown agent should succeed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// discoverSkills — deduplication via seen map
// ---------------------------------------------------------------------------

func TestDiscoverSkills_Deduplication(t *testing.T) {
	// The seen map prevents the same skillDir from being returned twice even
	// if buildDiscoveryDirs would list overlapping subdirectories.
	// Verify by placing a skill at the root "." which is always in the list.
	root := t.TempDir()
	d := filepath.Join(root, "dup-skill")
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("---\nname: dup-skill\n---\n"), 0o644)

	skills, err := discoverSkills(root)
	if err != nil {
		t.Fatalf("discoverSkills: %v", err)
	}
	count := 0
	for _, sk := range skills {
		if sk.Name == "dup-skill" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected dup-skill to appear once after dedup, got %d", count)
	}
}

func TestDiscoverSkills_RootSKILLmd(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: root-skill\n---\n"), 0o644)
	skills, err := discoverSkills(root)
	if err != nil {
		t.Fatalf("discoverSkills: %v", err)
	}
	found := false
	for _, sk := range skills {
		if sk.Name == "root-skill" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected root-skill from root SKILL.md; got %v", skills)
	}
}

// ---------------------------------------------------------------------------
// Manager.detectInstalledAgents — global scope
// ---------------------------------------------------------------------------

func TestDetectInstalledAgents_GlobalScope(t *testing.T) {
	// In global mode the home dir is the check target.
	// Claude global scope uses "~/.claude/skills" → parent is ~/.claude.
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	m := NewManager(nil, t.TempDir(), home, t.TempDir(), "", false)
	found := m.detectInstalledAgents(true)
	hadAgent := false
	for _, name := range found {
		if name == "claude-code" {
			hadAgent = true
		}
	}
	if !hadAgent {
		t.Errorf("expected claude-code detected in global mode when ~/.claude/ exists; found: %v", found)
	}
}

// ---------------------------------------------------------------------------
// Manager.syncAgents — the sync-time agent resolver.
//
// Wildcard ("*" or empty) → only installed agents (never creates dirs).
// Explicit list          → all named agents (sync creates dirs as needed).
// ---------------------------------------------------------------------------

func TestSyncAgents_ExplicitList_ProjectScope_ReturnsAll(t *testing.T) {
	workDir := t.TempDir()
	// claude-code has its dir, zencoder does not — both must be returned.
	os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755)

	m := NewManager(nil, t.TempDir(), "/home/testuser", workDir, "", false)
	got := m.syncAgents(config.ConfigSkill{
		Source: "owner/repo",
		Agents: []string{"claude-code", "zencoder"},
		Global: false,
	})

	if len(got) != 2 {
		t.Errorf("expected [claude-code zencoder], got %v", got)
	}
}

func TestSyncAgents_ExplicitList_GlobalScope_ReturnsAll(t *testing.T) {
	home := t.TempDir()
	// claude-code is "installed" globally, zencoder is not — both must be returned.
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	m := NewManager(nil, t.TempDir(), home, t.TempDir(), "", false)
	got := m.syncAgents(config.ConfigSkill{
		Source: "owner/repo",
		Agents: []string{"claude-code", "zencoder"},
		Global: true,
	})

	if len(got) != 2 {
		t.Errorf("expected [claude-code zencoder], got %v", got)
	}
}

func TestSyncAgents_ExplicitList_AllUninstalled_ReturnsAll(t *testing.T) {
	workDir := t.TempDir() // fresh, no agent dirs
	m := NewManager(nil, t.TempDir(), "/home/testuser", workDir, "", false)
	got := m.syncAgents(config.ConfigSkill{
		Source: "owner/repo",
		Agents: []string{"zencoder", "kilo"},
		Global: false,
	})
	if len(got) != 2 {
		t.Errorf("expected [zencoder kilo], got %v", got)
	}
}

func TestSyncAgents_Wildcard_StillWorks(t *testing.T) {
	// Regression guard: `agents: ["*"]` behaviour must be unchanged —
	// detectInstalledAgents already filters, so the extra syncAgents pass
	// is a no-op.
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755)
	m := NewManager(nil, t.TempDir(), "/home/testuser", workDir, "", false)
	got := m.syncAgents(config.ConfigSkill{
		Source: "owner/repo",
		Agents: []string{"*"},
		Global: false,
	})
	hadClaude := false
	for _, a := range got {
		if a == "claude-code" {
			hadClaude = true
		}
		if a == "zencoder" {
			t.Errorf("zencoder must not be in wildcard result: %v", got)
		}
	}
	if !hadClaude {
		t.Errorf("expected claude-code in wildcard result, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// toCloneURL — ssh:// prefix unchanged
// ---------------------------------------------------------------------------

func TestToCloneURL_SSH_Schema_Unchanged(t *testing.T) {
	url := "ssh://git@internal.corp/team/repo.git"
	if got := toCloneURL(url); got != url {
		t.Errorf("ssh:// URL should be unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// skillDirModified
// ---------------------------------------------------------------------------

func TestSkillDirModified_Clean(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	content := []byte("# SKILL\n")
	os.WriteFile(filepath.Join(src, "SKILL.md"), content, 0o644)
	os.WriteFile(filepath.Join(dst, "SKILL.md"), content, 0o644)

	if skillDirModified(src, dst) {
		t.Error("expected skillDirModified=false when content is identical")
	}
}

func TestSkillDirModified_Modified(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("original"), 0o644)
	os.WriteFile(filepath.Join(dst, "SKILL.md"), []byte("changed"), 0o644)

	if !skillDirModified(src, dst) {
		t.Error("expected skillDirModified=true when file content differs")
	}
}

func TestSkillDirModified_MissingDstFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("content"), 0o644)
	// dst is empty — file absent

	if !skillDirModified(src, dst) {
		t.Error("expected skillDirModified=true when destination file is missing")
	}
}

func TestSkillDirModified_EmptySrc(t *testing.T) {
	if skillDirModified(t.TempDir(), t.TempDir()) {
		t.Error("expected skillDirModified=false when source is empty")
	}
}

// ---------------------------------------------------------------------------
// Manager.Status — Modified propagation
// ---------------------------------------------------------------------------

func TestManager_Status_ModifiedSkill(t *testing.T) {
	// Source has skill-a with SKILL.md = "original".
	sourceDir := t.TempDir()
	skillSrc := filepath.Join(sourceDir, "skill-a")
	os.MkdirAll(skillSrc, 0o755)
	os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("original"), 0o644)

	// Installed copy has been modified by the user.
	workDir := t.TempDir()
	claudeSkillsDir := filepath.Join(workDir, ".claude", "skills")
	skillDst := filepath.Join(claudeSkillsDir, "skill-a")
	os.MkdirAll(skillDst, 0o755)
	os.WriteFile(filepath.Join(skillDst, "SKILL.md"), []byte("user modified"), 0o644)

	skills := []config.ConfigSkill{
		{Source: sourceDir, Agents: []string{"claude-code"}, Global: false},
	}
	m := NewManager(skills, t.TempDir(), "/home/user", workDir, "", false)
	statuses := m.Status(context.Background())

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	st := statuses[0]
	if st.Err != nil {
		t.Fatalf("unexpected error: %v", st.Err)
	}
	hasModified := false
	for _, n := range st.Modified {
		if n == "skill-a" {
			hasModified = true
		}
	}
	if !hasModified {
		t.Errorf("expected skill-a in Modified, got: %v", st.Modified)
	}
}

func TestManager_Status_CleanSkill(t *testing.T) {
	// Source and destination are identical — Modified should be empty.
	sourceDir := t.TempDir()
	content := []byte("---\nname: skill-clean\n---\n# body\n")
	skillSrc := filepath.Join(sourceDir, "skill-clean")
	os.MkdirAll(skillSrc, 0o755)
	os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), content, 0o644)

	workDir := t.TempDir()
	claudeSkillsDir := filepath.Join(workDir, ".claude", "skills")
	skillDst := filepath.Join(claudeSkillsDir, "skill-clean")
	os.MkdirAll(skillDst, 0o755)
	os.WriteFile(filepath.Join(skillDst, "SKILL.md"), content, 0o644)

	skills := []config.ConfigSkill{
		{Source: sourceDir, Agents: []string{"claude-code"}, Global: false},
	}
	m := NewManager(skills, t.TempDir(), "/home/user", workDir, "", false)
	statuses := m.Status(context.Background())

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	st := statuses[0]
	if len(st.Modified) != 0 {
		t.Errorf("expected Modified empty for clean skill, got: %v", st.Modified)
	}
}

// ---------------------------------------------------------------------------
// installSkill — error paths
// ---------------------------------------------------------------------------

func TestInstallSkill_MkdirAllFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission enforcement is not supported on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permissions — skipping")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# skill"), 0o644)

	err := installSkill(src, filepath.Join(parent, "new-subdir"))
	if err == nil {
		t.Fatal("expected error when destination parent is not writable")
	}
}

// ---------------------------------------------------------------------------
// copyFile — error paths
// ---------------------------------------------------------------------------

func TestCopyFile_SourceNotFound(t *testing.T) {
	err := copyFile("/nonexistent/source/file.txt", filepath.Join(t.TempDir(), "dst.txt"))
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestCopyFile_WriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission enforcement is not supported on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permissions — skipping")
	}
	src := t.TempDir()
	srcFile := filepath.Join(src, "file.txt")
	os.WriteFile(srcFile, []byte("content"), 0o644)

	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })

	err := copyFile(srcFile, filepath.Join(parent, "dst.txt"))
	if err == nil {
		t.Fatal("expected error when destination parent is not writable")
	}
}

// ---------------------------------------------------------------------------
// Prune
// ---------------------------------------------------------------------------

func TestPrune_RemovesOrphanSkill(t *testing.T) {
	// Source has two skills: kept-skill and orphan-skill.
	sourceDir := t.TempDir()
	for _, name := range []string{"kept-skill", "orphan-skill"} {
		dir := filepath.Join(sourceDir, name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644)
	}

	// Agent dir simulates an already-installed state: both skills are present.
	agentSkillsDir := t.TempDir()
	agentParent := filepath.Dir(agentSkillsDir)
	// workDir must have a subdirectory matching the agent's project_skills_dir.
	// Use a local-path skill with an absolute skillsDir to avoid agent registry lookup.
	for _, name := range []string{"kept-skill", "orphan-skill"} {
		os.MkdirAll(filepath.Join(agentSkillsDir, name), 0o755)
	}

	// Config only selects kept-skill.
	cfg := []config.ConfigSkill{
		{Source: sourceDir, Select: []string{"kept-skill"}, Agents: []string{"claude-code"}, Global: true},
	}

	// Build a manager whose home points to agentParent so that SkillDir
	// for claude-code global resolves to agentParent/.claude/skills.
	// We pre-create the directory and redirect home to agentParent.
	claudeSkillsDir := filepath.Join(agentParent, ".claude", "skills")
	os.MkdirAll(claudeSkillsDir, 0o755)
	for _, name := range []string{"kept-skill", "orphan-skill"} {
		os.MkdirAll(filepath.Join(claudeSkillsDir, name), 0o755)
	}

	m := NewManager(cfg, t.TempDir(), agentParent, t.TempDir(), "", false)
	if err := m.Prune(context.Background()); err != nil {
		t.Fatalf("Prune returned error: %v", err)
	}

	// orphan-skill must be gone, kept-skill must remain.
	if _, err := os.Stat(filepath.Join(claudeSkillsDir, "orphan-skill")); err == nil {
		t.Error("expected orphan-skill to be removed")
	}
	if _, err := os.Stat(filepath.Join(claudeSkillsDir, "kept-skill")); err != nil {
		t.Errorf("expected kept-skill to remain: %v", err)
	}
}

// TestPrune_RemovesOrphanSymlink covers the regression in #96: when an agent
// skills dir contains symlinks placed by a sibling tool (e.g. pointing into a
// shared `~/.agents/skills/` tree), Prune must clean those up alongside real
// dirs. The previous implementation only considered IsDir entries, so stale
// symlinks were never removed even when they were not in the configuration.
func TestPrune_RemovesOrphanSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin/Developer Mode on Windows; skip the regression test there")
	}
	sourceDir := t.TempDir()
	keptDir := filepath.Join(sourceDir, "kept-skill")
	os.MkdirAll(keptDir, 0o755)
	os.WriteFile(filepath.Join(keptDir, "SKILL.md"), []byte("---\nname: kept-skill\n---\n"), 0o644)

	home := t.TempDir()
	claudeSkillsDir := filepath.Join(home, ".claude", "skills")
	os.MkdirAll(claudeSkillsDir, 0o755)

	// Real install of the configured skill — must survive.
	os.MkdirAll(filepath.Join(claudeSkillsDir, "kept-skill"), 0o755)

	// Stale symlink placed by another tool — must be pruned.
	staleTarget := t.TempDir()
	os.MkdirAll(filepath.Join(staleTarget, "stale-symlinked"), 0o755)
	if err := os.Symlink(filepath.Join(staleTarget, "stale-symlinked"), filepath.Join(claudeSkillsDir, "stale-symlinked")); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}

	cfg := []config.ConfigSkill{
		{Source: sourceDir, Select: []string{"kept-skill"}, Agents: []string{"claude-code"}, Global: true},
	}
	m := NewManager(cfg, t.TempDir(), home, t.TempDir(), "", false)
	if err := m.Prune(context.Background()); err != nil {
		t.Fatalf("Prune returned error: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(claudeSkillsDir, "stale-symlinked")); !os.IsNotExist(err) {
		t.Errorf("expected stale-symlinked to be removed, got err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(claudeSkillsDir, "kept-skill")); err != nil {
		t.Errorf("expected kept-skill to remain, got err=%v", err)
	}
	// The symlink target must NOT have been removed — Prune deletes the
	// link, not the underlying directory.
	if _, err := os.Stat(filepath.Join(staleTarget, "stale-symlinked")); err != nil {
		t.Errorf("expected symlink target to remain on disk, got err=%v", err)
	}
}

func TestPrune_NoOpWhenAllManaged(t *testing.T) {
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644)

	home := t.TempDir()
	claudeSkillsDir := filepath.Join(home, ".claude", "skills")
	os.MkdirAll(filepath.Join(claudeSkillsDir, "my-skill"), 0o755)

	cfg := []config.ConfigSkill{
		{Source: sourceDir, Agents: []string{"claude-code"}, Global: true},
	}
	m := NewManager(cfg, t.TempDir(), home, t.TempDir(), "", false)
	if err := m.Prune(context.Background()); err != nil {
		t.Fatalf("Prune returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(claudeSkillsDir, "my-skill")); err != nil {
		t.Errorf("expected my-skill to remain: %v", err)
	}
}

// ── behaviour warnings (via internal/core/agent) ───────────────────────────

// captureSlog redirects slog output to a buffer for the duration of fn.
func captureSlog(t *testing.T, fn func()) string {
	t.Helper()
	var buf strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	fn()
	return buf.String()
}

// TestEmitBehaviorWarnings_FiresForExplicitClaudeDesktop is the regression
// for #208 / former TestWarnSkillsTargetingClaudeDesktop_FiresForExplicitTarget:
// declaring claude-desktop as a skill target must produce
// WarnSkillsUnsupported via the data-driven Behavior registry.
func TestEmitBehaviorWarnings_FiresForExplicitClaudeDesktop(t *testing.T) {
	skills := []config.ConfigSkill{
		{Source: "owner/repo", Agents: []string{"claude-desktop"}, Global: true},
	}
	out := captureSlog(t, func() {
		m := NewManager(skills, t.TempDir(), "/home/u", t.TempDir(), t.TempDir(), false)
		m.emitConfigWarnings()
	})
	if !strings.Contains(out, "code=skills_unsupported") {
		t.Errorf("expected skills_unsupported warning code in log, got: %s", out)
	}
	if !strings.Contains(out, "agent=claude-desktop") {
		t.Errorf("expected agent=claude-desktop attribute in log, got: %s", out)
	}
}

func TestEmitBehaviorWarnings_FiresForWildcardAgents(t *testing.T) {
	skills := []config.ConfigSkill{
		{Source: "owner/repo", Agents: []string{"*"}, Global: true},
	}
	out := captureSlog(t, func() {
		m := NewManager(skills, t.TempDir(), "/home/u", t.TempDir(), t.TempDir(), false)
		m.emitConfigWarnings()
	})
	if !strings.Contains(out, "code=skills_unsupported") || !strings.Contains(out, "agent=claude-desktop") {
		t.Errorf("expected wildcard expansion to surface claude-desktop skills_unsupported, got: %s", out)
	}
}

func TestEmitBehaviorWarnings_SilentForHappyAgents(t *testing.T) {
	skills := []config.ConfigSkill{
		{Source: "owner/repo", Agents: []string{"claude-code", "codex"}, Global: true},
		{Source: "owner/other", Agents: []string{"cursor"}, Global: false},
	}
	out := captureSlog(t, func() {
		m := NewManager(skills, t.TempDir(), "/home/u", t.TempDir(), t.TempDir(), false)
		m.emitConfigWarnings()
	})
	if strings.Contains(out, "code=skills_unsupported") {
		t.Errorf("expected no skills_unsupported warning for non-claude-desktop agents, got: %s", out)
	}
	if strings.Contains(out, "code=unsupported_platform") {
		t.Errorf("expected no platform warning for happy agents, got: %s", out)
	}
}

// TestEmitBehaviorWarnings_DedupesAcrossScopes is a regression for the
// pre-refactor behaviour where multiple skill entries targeting
// claude-desktop produced multiple identical log lines. The new system
// dedupes by (Code, Agent) regardless of how many entries match.
func TestEmitBehaviorWarnings_DedupesAcrossScopes(t *testing.T) {
	skills := []config.ConfigSkill{
		{Source: "a/x", Agents: []string{"claude-desktop"}, Global: true},
		{Source: "a/y", Agents: []string{"claude-desktop"}, Global: false},
		{Source: "a/z", Agents: []string{"claude-desktop"}, Global: false},
	}
	out := captureSlog(t, func() {
		m := NewManager(skills, t.TempDir(), "/home/u", t.TempDir(), t.TempDir(), false)
		m.emitConfigWarnings()
	})
	if got := strings.Count(out, "code=skills_unsupported"); got != 1 {
		t.Errorf("expected skills_unsupported to fire exactly once across scopes, fired %d times: %s", got, out)
	}
}

func TestEmitConfigWarnings_FiresOncePerManager(t *testing.T) {
	skills := []config.ConfigSkill{
		{Source: "owner/repo", Agents: []string{"claude-desktop"}, Global: true},
	}
	out := captureSlog(t, func() {
		m := NewManager(skills, t.TempDir(), "/home/u", t.TempDir(), t.TempDir(), false)
		// Both Sync and Status invoke emitConfigWarnings; the warning must
		// surface exactly once across however many calls.
		m.emitConfigWarnings()
		m.emitConfigWarnings()
		m.emitConfigWarnings()
	})
	count := strings.Count(out, "code=skills_unsupported")
	if count != 1 {
		t.Errorf("expected warning to fire exactly once, fired %d times", count)
	}
}

// TestManager_Sync_AggregatesErrorsAcrossSources is a regression for #110:
// before the fix, the first failing source aborted the loop; later sources
// were silently skipped. Now, per-source failures are accumulated via
// errors.Join and every source is attempted.
//
// Setup: workDir points at a regular file, so any MkdirAll for a
// project-scope skills dir fails. Two sources each carry a valid skill that
// targets the "claude-code" agent. Both should fail the same way; the
// aggregated error must mention both source paths to prove the loop did not
// bail after the first failure.
func TestManager_Sync_AggregatesErrorsAcrossSources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("t.TempDir() returns the long-form path but the mkdir error uses the 8.3 short name, so strings.Contains misses; needs EvalSymlinks or basename comparison (tracked in #231)")
	}
	tmp := t.TempDir()
	workDirAsFile := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(workDirAsFile, []byte("file"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mkSource := func(name string) string {
		root := filepath.Join(tmp, name)
		skillDir := filepath.Join(root, name)
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
			[]byte("---\nname: "+name+"\n---\n"), 0o644)
		return root
	}
	sourceA := mkSource("source-a")
	sourceB := mkSource("source-b")

	// Force the project-scope MkdirAll path: agents:["claude-code"] with
	// global=false uses workDir/<.claude/skills>. The "force" flag at the
	// end bypasses the IsAgentInstalled gate.
	skills := []config.ConfigSkill{
		{Source: sourceA, Agents: []string{"claude-code"}},
		{Source: sourceB, Agents: []string{"claude-code"}},
	}
	m := NewManager(skills, t.TempDir(), tmp, workDirAsFile, "", true)

	err := m.Sync(context.Background())
	if err == nil {
		t.Fatal("expected aggregated error from MkdirAll failure on both sources, got nil")
	}
	if !strings.Contains(err.Error(), sourceA) {
		t.Errorf("aggregated error must reference source A; got: %v", err)
	}
	if !strings.Contains(err.Error(), sourceB) {
		t.Errorf("aggregated error must reference source B (proves loop continued past A); got: %v", err)
	}
}

// TestInstallSkill_DoesNotDereferenceSymlink is the regression for #113:
// a malicious skill source containing a symlink pointing at /etc/passwd
// previously had copyFile read the link target and write its contents into
// the install dir under the gaal-managed name. The new behavior skips
// symlinks (and other non-regular entries) with a warn.
func TestInstallSkill_DoesNotDereferenceSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	src := t.TempDir()
	// A regular file that should be installed normally.
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A "secret" target outside src that the malicious symlink would expose.
	secretDir := t.TempDir()
	secretPath := filepath.Join(secretDir, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("PRIVATE-DATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The malicious symlink inside src.
	if err := os.Symlink(secretPath, filepath.Join(src, "leak.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "installed")
	if err := installSkill(src, dst); err != nil {
		t.Fatalf("installSkill: %v", err)
	}

	// SKILL.md must be installed.
	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		t.Errorf("expected SKILL.md in dst: %v", err)
	}
	// leak.txt must NOT exist in dst (neither as a copy of secret content nor
	// as a recreated symlink).
	leakDst := filepath.Join(dst, "leak.txt")
	if data, err := os.ReadFile(leakDst); err == nil {
		t.Errorf("symlink leak: dst contains leak.txt with content %q (should not exist)", data)
	}
	if _, err := os.Lstat(leakDst); err == nil {
		t.Errorf("symlink leak: dst contains leak.txt entry (should not exist)")
	}
}
