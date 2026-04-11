package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

func TestResolveSkillSource_CacheReverseLookup(t *testing.T) {
	cacheRoot := t.TempDir()
	skillsRoot := filepath.Join(cacheRoot, "gaal", "skills")
	skillDir := filepath.Join(skillsRoot, "anthropics", "skills", "frontend-design")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	src, kind := ResolveSkillSource(skillDir, cacheRoot)
	if src != "anthropics/skills" {
		t.Errorf("source: got %q, want anthropics/skills", src)
	}
	if kind != SourceKindCache {
		t.Errorf("kind: got %q, want cache", kind)
	}
}

func TestResolveSkillSource_CacheMalformed(t *testing.T) {
	// A path under the cache root but with fewer than owner/repo segments
	// must fall through to git/path resolution.
	cacheRoot := t.TempDir()
	skillsRoot := filepath.Join(cacheRoot, "gaal", "skills")
	// only one segment under skills/ — malformed.
	shallow := filepath.Join(skillsRoot, "orphan")
	if err := os.MkdirAll(shallow, 0o755); err != nil {
		t.Fatal(err)
	}

	src, kind := ResolveSkillSource(shallow, cacheRoot)
	if kind == SourceKindCache {
		t.Errorf("should not report cache for malformed layout, got %q", src)
	}
	if src != shallow {
		t.Errorf("expected raw path fallback, got %q", src)
	}
	if kind != SourceKindPath {
		t.Errorf("kind: got %q, want path", kind)
	}
}

func TestResolveSkillSource_GitParent(t *testing.T) {
	cacheRoot := t.TempDir()
	repoDir := t.TempDir()

	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/example/my-skills.git"},
	}); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(repoDir, "packs", "lint")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	src, kind := ResolveSkillSource(skillDir, cacheRoot)
	if kind != SourceKindGit {
		t.Errorf("kind: got %q, want git", kind)
	}
	if src != "https://github.com/example/my-skills.git" {
		t.Errorf("source: got %q, want the origin URL", src)
	}
}

func TestResolveSkillSource_GitNoOrigin(t *testing.T) {
	cacheRoot := t.TempDir()
	repoDir := t.TempDir()
	if _, err := git.PlainInit(repoDir, false); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(repoDir, "skills", "local")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	src, kind := ResolveSkillSource(skillDir, cacheRoot)
	if kind != SourceKindPath {
		t.Errorf("kind: got %q, want path", kind)
	}
	if src != skillDir {
		t.Errorf("source: got %q, want %q", src, skillDir)
	}
}

func TestResolveSkillSource_OrphanPath(t *testing.T) {
	cacheRoot := t.TempDir()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "loose-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	src, kind := ResolveSkillSource(skillDir, cacheRoot)
	if kind != SourceKindPath {
		t.Errorf("kind: got %q, want path", kind)
	}
	if src != skillDir {
		t.Errorf("source: got %q, want raw path", src)
	}
}
