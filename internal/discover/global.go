package discover

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"gaal/internal/core/agent"
	"gaal/internal/core/vcs"
)

// scanGlobal discovers skill and MCP config resources at predictable
// agent-registry paths.
//
// A single call to agent.List() is used across both passes to avoid
// redundant iteration:
//   - Pass 1: canonical dirs (ProjectSkillsDir / GlobalSkillsDir) + MCP
//     config files — builds the dedup set with correct ownership.
//   - Pass 2: full extended search lists, skipping already-attributed dirs.
func scanGlobal(ctx context.Context, home, workDir, stateDir string) ([]Resource, error) {
	slog.DebugContext(ctx, "scanning global agent resource paths", "home", home)

	seenSkill := make(map[string]struct{})
	seenMCP := make(map[string]struct{})
	var resources []Resource

	agents := agent.List()

	// Pass 1: canonical install dirs + MCP config files.
	for _, a := range agents {
		if a.Info.ProjectSkillsDir != "" {
			dir := filepath.Join(workDir, a.Info.ProjectSkillsDir)
			resources = append(resources, skillsFromDir(ctx, dir, ScopeWorkspace, a.Name, stateDir, "project", seenSkill)...)
		}
		if a.Info.GlobalSkillsDir != "" {
			dir := agent.ExpandHome(a.Info.GlobalSkillsDir, home)
			resources = append(resources, skillsFromDir(ctx, dir, ScopeGlobal, a.Name, stateDir, "global", seenSkill)...)
		}
		// MCP config files are collected here alongside canonical skill dirs
		// so that a single agent.List() iteration covers both resource types.
		if cfgFile, ok := agent.ProjectMCPConfigPath(a.Name, home); ok {
			resources = appendMCPResource(resources, seenMCP, a.Name, cfgFile, ScopeWorkspace, stateDir)
		}
		if cfgFile, ok := agent.GlobalMCPConfigPath(a.Name, home); ok {
			resources = appendMCPResource(resources, seenMCP, a.Name, cfgFile, ScopeGlobal, stateDir)
		}
	}

	// Pass 2: extended search lists (skills only; MCPs use direct paths, no extended search).
	for _, a := range agents {
		for _, rel := range agent.ExpandedProjectSkillsSearch(a.Name) {
			dir := filepath.Join(workDir, rel)
			resources = append(resources, skillsFromDir(ctx, dir, ScopeWorkspace, a.Name, stateDir, "project", seenSkill)...)
		}
		for _, abs := range agent.ExpandedGlobalSkillsSearch(a.Name, home) {
			resources = append(resources, skillsFromDir(ctx, abs, ScopeGlobal, a.Name, stateDir, "global", seenSkill)...)
		}
		for _, pmRoot := range agent.ExpandedPmSkillsSearch(a.Name, home) {
			skillDirs, err := walkSkillDirs(pmRoot)
			if err != nil {
				slog.DebugContext(ctx, "pm walk error", "agent", a.Name, "root", pmRoot, "err", err)
				continue
			}
			for _, sd := range skillDirs {
				resources = append(resources, skillsFromDir(ctx, sd, ScopeGlobal, a.Name, stateDir, "package-manager", seenSkill)...)
			}
		}
	}

	return resources, nil
}

// skillsFromDir scans dir at one level deep and returns a Resource for each
// subdirectory that contains a SKILL.md file. dir itself is also checked.
// Directories already present in seen are skipped.
func skillsFromDir(ctx context.Context, dir string, scope Scope, agentName, stateDir, source string, seen map[string]struct{}) []Resource {
	var out []Resource

	add := func(skillDir string) {
		if _, ok := seen[skillDir]; ok {
			return
		}
		seen[skillDir] = struct{}{}
		slog.DebugContext(ctx, "adding skill resource", "path", skillDir, "agent", agentName, "source", source)
		name := skillName(skillDir)
		drift := computeSkillDrift(ctx, skillDir, stateDir)
		_, desc, _ := parseSkillFrontmatter(filepath.Join(skillDir, "SKILL.md"))
		out = append(out, Resource{
			Type:  ResourceSkill,
			Scope: scope,
			Path:  skillDir,
			Name:  name,
			Drift: drift,
			Meta: map[string]string{
				"agent":  agentName,
				"source": source,
				"desc":   desc,
			},
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
//
// Uses yaml.v3 on the frontmatter slice (#133). Tolerates CRLF, quoted
// values, and values containing ":" — all of which the previous
// strings.Cut(":") implementation silently mangled.
func parseSkillFrontmatter(path string) (name, desc string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	fm := extractDiscoverFrontmatter(data)
	if len(fm) == 0 {
		return "", "", nil
	}
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		// Bad frontmatter degrades to empty — caller falls back to dir name.
		return "", "", nil
	}
	return meta.Name, meta.Description, nil
}

// extractDiscoverFrontmatter mirrors skill.extractFrontmatter (kept here
// to avoid an import cycle: discover is imported by skill, not vice
// versa).
func extractDiscoverFrontmatter(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	in := false
	var fm bytes.Buffer
	for _, raw := range lines {
		line := bytes.TrimRight(raw, "\r")
		if string(line) == "---" {
			if !in {
				in = true
				continue
			}
			return fm.Bytes()
		}
		if in {
			fm.Write(line)
			fm.WriteByte('\n')
		}
	}
	return nil
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
