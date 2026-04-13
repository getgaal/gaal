package discover

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gaal/internal/core/vcs"
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
