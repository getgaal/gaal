package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gaal/internal/config"
)

func TestToCloneURL_GitHubShorthand(t *testing.T) {
	got := toCloneURL("owner/repo")
	want := "https://github.com/owner/repo"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestToCloneURL_HTTPS_Unchanged(t *testing.T) {
	url := "https://gitlab.com/user/project.git"
	if got := toCloneURL(url); got != url {
		t.Errorf("HTTPS URL should be unchanged, got %q", got)
	}
}

func TestToCloneURL_SSH_Unchanged(t *testing.T) {
	url := "git@github.com:user/repo.git"
	if got := toCloneURL(url); got != url {
		t.Errorf("SSH URL should be unchanged, got %q", got)
	}
}

func TestURLToCacheKey_HTTPS(t *testing.T) {
	key := urlToCacheKey("https://github.com/owner/repo.git")
	if key == "" {
		t.Fatal("expected non-empty cache key")
	}
	for _, c := range []byte(key) {
		if c == ':' {
			t.Errorf("cache key should not contain ':', got %q", key)
		}
	}
}

func TestURLToCacheKey_DifferentURLs_DifferentKeys(t *testing.T) {
	key1 := urlToCacheKey("https://github.com/owner/repo1")
	key2 := urlToCacheKey("https://github.com/owner/repo2")
	if key1 == key2 {
		t.Error("expected different cache keys for different URLs")
	}
}

func TestCachedSourcePath_LocalPath(t *testing.T) {
	path, err := cachedSourcePath(t.TempDir(), "/abs/local/path")
	if err != nil {
		t.Fatalf("cachedSourcePath local: %v", err)
	}
	if path != "/abs/local/path" {
		t.Errorf("expected local path unchanged, got %q", path)
	}
}

func TestCachedSourcePath_NotCached(t *testing.T) {
	path, err := cachedSourcePath(t.TempDir(), "owner/repo-not-cached")
	if err != nil {
		t.Fatalf("cachedSourcePath: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path for not-yet-cached source, got %q", path)
	}
}

func TestCachedSourcePath_Cached(t *testing.T) {
	cacheDir := t.TempDir()
	cloneURL := toCloneURL("owner/cached-repo")
	cacheKey := urlToCacheKey(cloneURL)
	cachePath := filepath.Join(cacheDir, cacheKey)
	os.MkdirAll(cachePath, 0o755)
	path, err := cachedSourcePath(cacheDir, "owner/cached-repo")
	if err != nil {
		t.Fatalf("cachedSourcePath: %v", err)
	}
	if path != cachePath {
		t.Errorf("got %q, want %q", path, cachePath)
	}
}

func TestManager_Sync_LocalSource_WithSkills(t *testing.T) {
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "cool-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: cool-skill\n---\n"), 0o644)
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755)
	skills := []config.ConfigSkill{
		{Source: sourceDir, Agents: []string{"claude-code"}, Global: false},
	}
	m := NewManager(skills, t.TempDir(), "/home/user", workDir, "")
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	destDir := filepath.Join(workDir, ".claude", "skills", "cool-skill")
	if _, err := os.Stat(destDir); err != nil {
		t.Errorf("expected skill installed at %q: %v", destDir, err)
	}
}

func TestManager_Sync_NoSkillsFound(t *testing.T) {
	sourceDir := t.TempDir()
	skills := []config.ConfigSkill{
		{Source: sourceDir, Agents: []string{"claude-code"}},
	}
	m := NewManager(skills, t.TempDir(), "/home/user", t.TempDir(), "")
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync with no skills: %v", err)
	}
}

func TestResolveAgents_ExplicitList(t *testing.T) {
	m := NewManager(nil, t.TempDir(), "/home/user", t.TempDir(), "")
	sc := config.ConfigSkill{Agents: []string{"claude-code", "cursor"}}
	agents := m.resolveAgents(sc)
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d: %v", len(agents), agents)
	}
}

func TestResolveAgents_Wildcard(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755)
	m := NewManager(nil, t.TempDir(), "/home/user", workDir, "")
	sc := config.ConfigSkill{Agents: []string{"*"}}
	agents := m.resolveAgents(sc)
	found := false
	for _, a := range agents {
		if a == "claude-code" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected claude-code to be detected, got: %v", agents)
	}
}

func TestResolveAgents_Empty(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".roo"), 0o755)
	m := NewManager(nil, t.TempDir(), "/home/user", workDir, "")
	sc := config.ConfigSkill{Agents: nil}
	agents := m.resolveAgents(sc)
	found := false
	for _, a := range agents {
		if a == "roo" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected roo to be detected when .roo/ exists, got: %v", agents)
	}
}

func TestManager_Status_Empty(t *testing.T) {
	m := NewManager(nil, t.TempDir(), "/home/user", t.TempDir(), "")
	statuses := m.Status(context.Background())
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses for empty manager, got %d", len(statuses))
	}
}

// ---------------------------------------------------------------------------
// resolveSource — local git repos are updated on sync
// ---------------------------------------------------------------------------

func TestResolveSource_LocalNonGit_ReturnedAsIs(t *testing.T) {
	// A plain directory (no .git) must be returned without any git operation.
	src := t.TempDir()
	m := NewManager(nil, t.TempDir(), "/home/user", t.TempDir(), "")
	got, err := m.resolveSource(context.Background(), src)
	if err != nil {
		t.Fatalf("resolveSource: %v", err)
	}
	if got != src {
		t.Errorf("expected path %q, got %q", src, got)
	}
}

func TestResolveSource_LocalGit_UpdateAttempted(t *testing.T) {
	// A local directory that contains a .git folder: update is attempted.
	// Since there is no real remote the fetch will fail; resolveSource must
	// still return the path (the failure is only a warning).
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, ".git"), 0o755)
	m := NewManager(nil, t.TempDir(), "/home/user", t.TempDir(), "")
	// The git binary must be present; skip on machines without git in PATH.
	got, err := m.resolveSource(context.Background(), src)
	if err != nil {
		t.Fatalf("resolveSource should not fail for a local git dir: %v", err)
	}
	if got != src {
		t.Errorf("expected path %q unchanged, got %q", src, got)
	}
}

// ---------------------------------------------------------------------------
// Manager.resolveSource — pre-cached remote source (update branch)
// ---------------------------------------------------------------------------

func TestManager_Sync_PreCachedRemoteSource(t *testing.T) {
	// Compute the cache path that would be used for "owner/pre-cached".
	cacheDir := t.TempDir()
	cloneURL := toCloneURL("owner/pre-cached")
	cacheKey := urlToCacheKey(cloneURL)
	localPath := filepath.Join(cacheDir, cacheKey)

	// Pre-populate .git so gitVCS.isCloned returns true.
	os.MkdirAll(filepath.Join(localPath, ".git"), 0o755)

	// Also create a skill inside so syncOne has something to discover.
	skillDir := filepath.Join(localPath, "demo-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo-skill\n---\n"), 0o644)

	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755)

	skills := []config.ConfigSkill{
		{Source: "owner/pre-cached", Agents: []string{"claude-code"}, Global: false},
	}
	m := NewManager(skills, cacheDir, "/home/user", workDir, "")

	// git update will fail (not a real repo) but the error is swallowed by resolveSource.
	// Sync should still succeed.
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync with pre-cached source: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Manager.resolveSource — clone error path (binary missing)
// ---------------------------------------------------------------------------

func TestManager_Sync_CloneError(t *testing.T) {
	// Use a URL that will fail immediately (connection refused on localhost:1).
	skills := []config.ConfigSkill{
		{Source: "http://localhost:1/not-a-repo.git", Agents: []string{"claude-code"}},
	}
	m := NewManager(skills, t.TempDir(), "/home/user", t.TempDir(), "")
	err := m.Sync(context.Background())
	if err == nil {
		t.Fatal("expected error when git clone fails (unreachable host)")
	}
}
