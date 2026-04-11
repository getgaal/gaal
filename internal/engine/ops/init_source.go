package ops

import (
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
)

// SourceKind describes how the source of a skill was resolved.
type SourceKind string

const (
	// SourceKindCache means the path lives under $UserCacheDir/gaal/skills
	// and the source was reconstructed as "owner/repo".
	SourceKindCache SourceKind = "cache"
	// SourceKindGit means the nearest parent .git directory exposed an
	// "origin" remote whose URL is used as the source.
	SourceKindGit SourceKind = "git"
	// SourceKindPath means no better information was available and the raw
	// disk path is returned as-is.
	SourceKindPath SourceKind = "path"
)

// ResolveSkillSource converts a concrete skill directory into the best
// "source" string for a gaal.yaml SkillConfig entry.
//
// Resolution order, documented in the spec:
//
//  1. If skillDir lives under cacheRoot/gaal/skills/<owner>/<repo>/..., return
//     "owner/repo", kind cache.
//  2. Otherwise, if a parent directory is a git working copy with an origin
//     remote, return that origin URL, kind git.
//  3. Otherwise, return skillDir unchanged, kind path.
//
// ResolveSkillSource never returns an error: every failure falls through to
// the next strategy and ultimately to the path fallback.
func ResolveSkillSource(skillDir, cacheRoot string) (string, SourceKind) {
	slog.Debug("resolving skill source", "dir", skillDir, "cacheRoot", cacheRoot)

	if src, ok := reverseLookupCache(skillDir, cacheRoot); ok {
		return src, SourceKindCache
	}
	if src, ok := lookupGitOrigin(skillDir); ok {
		return src, SourceKindGit
	}
	return skillDir, SourceKindPath
}

// reverseLookupCache returns "owner/repo" when skillDir lives under
// cacheRoot/gaal/skills/<owner>/<repo>/... — matching the layout written by
// skill.Manager.
func reverseLookupCache(skillDir, cacheRoot string) (string, bool) {
	if cacheRoot == "" {
		return "", false
	}
	skillsRoot := filepath.Join(cacheRoot, "gaal", "skills")
	rel, err := filepath.Rel(skillsRoot, skillDir)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "..") || rel == "." {
		return "", false
	}
	parts := strings.Split(rel, "/")
	if len(parts) < 2 {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

// lookupGitOrigin walks upward from skillDir, opens the first git repository
// it finds, and returns the URL of the "origin" remote when present.
func lookupGitOrigin(skillDir string) (string, bool) {
	dir := skillDir
	for {
		if repo, err := git.PlainOpen(dir); err == nil {
			remote, err := repo.Remote("origin")
			if err != nil {
				return "", false
			}
			urls := remote.Config().URLs
			if len(urls) == 0 {
				return "", false
			}
			return urls[0], true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
