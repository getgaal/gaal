package discover

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getgaal/gaal/internal/core/vcs"
)

// skipDirs is the set of directory names that the workspace walk never descends into.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	".cache":       true,
	"bin":          true,
}

// scanWorkspace discovers repos and skills inside workDir using a depth-limited
// FS walk. It respects context cancellation (e.g. from the scan timeout).
//
// Rules:
//   - Directories containing a VCS marker (.git, .hg, .svn, .bzr) are
//     reported as repos; the walk does not descend into them.
//   - The workDir root itself is never reported as a repo (it is the project
//     workspace, not a gaal-managed clone).
//   - Directories containing SKILL.md are reported as skills.
//   - Walks stops when reaching maxDepth levels below workDir.
func scanWorkspace(ctx context.Context, workDir string, maxDepth int, stateDir string) ([]Resource, error) {
	slog.DebugContext(ctx, "scanning workspace", "dir", workDir, "maxDepth", maxDepth)

	root := filepath.Clean(workDir)
	seen := make(map[string]struct{})
	var resources []Resource

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		depth := depthOf(rel)
		if depth > maxDepth {
			return filepath.SkipDir
		}

		base := filepath.Base(path)
		if skipDirs[base] && path != root {
			return filepath.SkipDir
		}

		// Never report the workspace root itself as a repo.
		if path != root && hasVCSMarker(path) {
			if _, ok := seen[path]; !ok {
				seen[path] = struct{}{}
				vcsType := vcs.DetectType(path)
				resources = append(resources, Resource{
					Type:    ResourceRepo,
					Scope:   ScopeWorkspace,
					Path:    path,
					Name:    base,
					Drift:   computeRepoDrift(ctx, path, vcsType),
					VCSType: vcsType,
				})
			}
			return filepath.SkipDir
		}

		// Skill detection: directory contains SKILL.md.
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
			if _, ok := seen[path]; !ok {
				seen[path] = struct{}{}
				name := skillName(path)
				resources = append(resources, Resource{
					Type:  ResourceSkill,
					Scope: ScopeWorkspace,
					Path:  path,
					Name:  name,
					Drift: computeSkillDrift(ctx, path, stateDir),
				})
			}
		}

		return nil
	})

	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		return nil, err
	}
	return resources, nil
}

// computeRepoDrift checks drift for a VCS repository using the native backend.
func computeRepoDrift(ctx context.Context, dir, vcsType string) DriftState {
	if vcsType == "" {
		return DriftUnknown
	}
	backend, err := vcs.New(vcsType)
	if err != nil {
		return DriftUnknown
	}
	if !backend.IsCloned(dir) {
		return DriftMissing
	}
	changed, err := backend.HasChanges(ctx, dir)
	if err != nil {
		return DriftUnknown
	}
	if changed {
		return DriftModified
	}
	return DriftOK
}

// depthOf returns the number of path separators in a relative path string.
func depthOf(rel string) int {
	if rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// ImmediateVCSChildren returns the base names of every immediate child
// directory of workDir that contains a VCS marker (.git, .hg, .svn, .bzr),
// sorted alphabetically for deterministic output. It does not recurse —
// only direct children of workDir are checked, matching the flat
// umbrella-directory-of-repos layout (one directory holding many
// independent repo clones as siblings) this is built for. skipDirs entries
// (node_modules, vendor, …) are skipped like the rest of workspace
// scanning. A workDir that does not exist or cannot be read returns an
// empty, non-error result — callers that need to distinguish "no repos
// found" from "workDir itself is missing" should stat workDir first.
func ImmediateVCSChildren(workDir string) ([]string, error) {
	slog.Debug("scanning for immediate VCS children", "workDir", workDir)
	entries, err := os.ReadDir(workDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var children []string
	for _, e := range entries {
		if !e.IsDir() || skipDirs[e.Name()] {
			continue
		}
		if hasVCSMarker(filepath.Join(workDir, e.Name())) {
			children = append(children, e.Name())
		}
	}
	sort.Strings(children)
	return children, nil
}

// ResolveRoots expands a roots: declaration (from ConfigMcp, ConfigSkill, or
// ConfigContentTarget — the field means the same thing on all three) into
// the concrete list of workspace-relative sub-repo directories a
// project-scope entry fans out across. An entry naming "*" anywhere in the
// list is replaced by ImmediateVCSChildren(workDir); every other entry is
// kept as a literal workspace-relative root. Results are deduplicated,
// preserving first-seen order so config-declared roots sort before
// discovered ones. Shared by every manager package (mcp, skill, content)
// so "roots:" resolves identically everywhere it appears.
func ResolveRoots(declared []string, workDir string) []string {
	slog.Debug("resolving roots", "declared", declared, "workDir", workDir)
	seen := make(map[string]struct{}, len(declared))
	var out []string
	add := func(r string) {
		if _, ok := seen[r]; ok {
			return
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	for _, r := range declared {
		if r == "*" {
			discovered, err := ImmediateVCSChildren(workDir)
			if err != nil {
				slog.Warn("roots auto-discovery failed", "workDir", workDir, "err", err)
				continue
			}
			for _, d := range discovered {
				add(d)
			}
			continue
		}
		add(r)
	}
	return out
}
