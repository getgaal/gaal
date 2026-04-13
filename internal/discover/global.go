package discover

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gaal/internal/core/agent"
	"gaal/internal/core/vcs"
)

// scanGlobal discovers skill resources at predictable agent-registry paths.
//
// It mirrors the two-pass attribution strategy from ops/audit.go:
//   - Pass 1: canonical dirs only (ProjectSkillsDir / GlobalSkillsDir).
//   - Pass 2: full search lists, skipping already-attributed directories.
func scanGlobal(ctx context.Context, home, workDir, stateDir string) ([]Resource, error) {
	slog.DebugContext(ctx, "scanning global skill paths", "home", home)

	seen := make(map[string]struct{})
	var resources []Resource

	agents := agent.List()

	// Pass 1: canonical install dirs.
	for _, a := range agents {
		if a.Info.ProjectSkillsDir != "" {
			dir := filepath.Join(workDir, a.Info.ProjectSkillsDir)
			resources = append(resources, skillsFromDir(ctx, dir, ScopeWorkspace, a.Name, stateDir, seen)...)
		}
		if a.Info.GlobalSkillsDir != "" {
			dir := agent.ExpandHome(a.Info.GlobalSkillsDir, home)
			resources = append(resources, skillsFromDir(ctx, dir, ScopeGlobal, a.Name, stateDir, seen)...)
		}
	}

	// Pass 2: extended search lists.
	for _, a := range agents {
		for _, rel := range agent.ExpandedProjectSkillsSearch(a.Name) {
			dir := filepath.Join(workDir, rel)
			resources = append(resources, skillsFromDir(ctx, dir, ScopeWorkspace, a.Name, stateDir, seen)...)
		}
		for _, abs := range agent.ExpandedGlobalSkillsSearch(a.Name, home) {
			resources = append(resources, skillsFromDir(ctx, abs, ScopeGlobal, a.Name, stateDir, seen)...)
		}
		for _, pmRoot := range agent.ExpandedPmSkillsSearch(a.Name, home) {
			skillDirs, err := walkSkillDirs(pmRoot)
			if err != nil {
				slog.DebugContext(ctx, "pm walk error", "agent", a.Name, "root", pmRoot, "err", err)
				continue
			}
			for _, sd := range skillDirs {
				resources = append(resources, skillsFromDir(ctx, sd, ScopeGlobal, a.Name, stateDir, seen)...)
			}
		}
	}

	return resources, nil
}

// skillsFromDir scans dir at one level deep and returns a Resource for each
// subdirectory that contains a SKILL.md file. dir itself is also checked.
// Directories already present in seen are skipped.
func skillsFromDir(ctx context.Context, dir string, scope Scope, agentName, stateDir string, seen map[string]struct{}) []Resource {
	var out []Resource

	add := func(skillDir string) {
		if _, ok := seen[skillDir]; ok {
			return
		}
		seen[skillDir] = struct{}{}
		name := skillName(skillDir)
		drift := computeSkillDrift(ctx, skillDir, stateDir)
		out = append(out, Resource{
			Type:  ResourceSkill,
			Scope: scope,
			Path:  skillDir,
			Name:  name,
			Drift: drift,
			Meta:  map[string]string{"agent": agentName},
		})
	}

	// dir itself may be a skill root.
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
		add(dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err == nil {
			add(skillDir)
		}
	}

	return out
}

// computeSkillDrift determines DriftState for an installed skill directory.
// VCS-native detection is preferred when the directory is a VCS working copy;
// snapshot-based diff is used as fallback.
func computeSkillDrift(ctx context.Context, dir, stateDir string) DriftState {
	// VCS-native: equivalent of "git status" on the skill directory.
	if hasVCSMarker(dir) {
		vcsType := vcs.DetectType(dir)
		backend, err := vcs.New(vcsType)
		if err == nil && backend.IsCloned(dir) {
			changed, err := backend.HasChanges(ctx, dir)
			if err != nil {
				slog.DebugContext(ctx, "vcs drift check failed", "dir", dir, "err", err)
				return DriftUnknown
			}
			if changed {
				return DriftModified
			}
			return DriftOK
		}
	}

	// Snapshot-based fallback.
	if stateDir == "" {
		return DriftUnknown
	}
	key := "skill-" + WorkdirKey(dir)
	snap, err := Load(SnapshotPath(stateDir, key))
	if err != nil || len(snap) == 0 {
		return DriftUnknown
	}
	changes, err := DiffPath(dir, snap)
	if err != nil || len(changes) == 0 {
		return DriftOK
	}
	for _, c := range changes {
		if c.Drift == DriftMissing {
			return DriftMissing
		}
	}
	return DriftModified
}

// skillName extracts a human-readable skill name from a directory.
// It parses the SKILL.md frontmatter when available, falling back to the
// directory base name.
func skillName(dir string) string {
	name, _, _ := parseSkillFrontmatter(filepath.Join(dir, "SKILL.md"))
	if name != "" {
		return name
	}
	return filepath.Base(dir)
}

// parseSkillFrontmatter reads the "name" and "description" fields from the
// YAML frontmatter block (--- … ---) of a SKILL.md file.
// Returns empty strings on any error; the caller must use a sensible fallback.
func parseSkillFrontmatter(path string) (name, desc string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	inFrontmatter := false
	for sc.Scan() {
		line := sc.Text()
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break
		}
		if !inFrontmatter {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "name":
			name = strings.TrimSpace(v)
		case "description":
			desc = strings.TrimSpace(v)
		}
	}
	return name, desc, sc.Err()
}

// walkSkillDirs recursively finds directories named "skills" under root.
// Each found "skills" directory is appended to the result; the walk does not
// descend into them.
func walkSkillDirs(root string) ([]string, error) {
	slog.Debug("walking for skill directories", "root", root)
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "skills" {
			dirs = append(dirs, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return dirs, nil
}

// hasVCSMarker reports whether dir contains at least one VCS metadata directory.
func hasVCSMarker(dir string) bool {
	for _, marker := range []string{".git", ".hg", ".svn", ".bzr"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}
