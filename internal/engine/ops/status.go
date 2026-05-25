package ops

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gaal/internal/content"
	"gaal/internal/discover"
	"gaal/internal/engine/render"
	"gaal/internal/mcp"
	"gaal/internal/repo"
	"gaal/internal/skill"
)

// Collect gathers the current status of all resources without side effects.
// It performs a FS-first scan via discover.Scan and then reconciles with any
// config-declared resources from the managers, marking them as managed.
func Collect(ctx context.Context, repos *repo.Manager, skills *skill.Manager, contentMgr *content.Manager, mcps *mcp.Manager, home, workDir, stateDir string) (*render.StatusReport, error) {
	slog.DebugContext(ctx, "collecting status", "home", home, "workDir", workDir)

	// FS-first: discover what is actually installed.
	discovered, err := discover.Scan(ctx, home, workDir, discover.ScanOptions{
		IncludeWorkspace: true,
		StateDir:         stateDir,
	})
	if err != nil {
		slog.DebugContext(ctx, "discover scan error", "err", err)
	}

	agents, err := collectAgents()
	if err != nil {
		return nil, err
	}

	// Config-driven status (may add managed resources absent from FS scan).
	configRepos := collectRepos(repos.Status(ctx))
	configSkills := collectSkills(skills.Status(ctx))
	configContent := collectContent(contentMgr.Status(ctx))
	configMCPs := collectMCPs(mcps.Status(ctx))

	// Reconcile: mark config-declared resources as managed and merge
	// FS-discovered unmanaged resources into the report.
	repoEntries := reconcileRepos(configRepos, discovered)
	skillEntries := reconcileSkills(configSkills, discovered, home, workDir, skills.SourcePaths())
	mcpEntries := reconcileMCPs(configMCPs, discovered)

	return &render.StatusReport{
		Repositories: repoEntries,
		Skills:       skillEntries,
		Content:      configContent,
		MCPs:         mcpEntries,
		Agents:       agents,
	}, nil
}

// Status collects the current resource state and renders it to os.Stdout.
func Status(ctx context.Context, repos *repo.Manager, skills *skill.Manager, contentMgr *content.Manager, mcps *mcp.Manager, home, workDir, stateDir string, format render.OutputFormat) error {
	slog.DebugContext(ctx, "status requested", "format", format)

	report, err := Collect(ctx, repos, skills, contentMgr, mcps, home, workDir, stateDir)
	if err != nil {
		return err
	}

	renderer, err := render.NewRenderer(format)
	if err != nil {
		return err
	}

	return renderer.Render(os.Stdout, report)
}

// reconcileRepos merges config-driven repo entries with FS-discovered repos.
// Config entries are kept as-is (already enriched with URL, version, etc.).
// FS-discovered repos not in config are appended as unmanaged.
func reconcileRepos(config []render.RepoEntry, resources []discover.Resource) []render.RepoEntry {
	known := make(map[string]struct{}, len(config))
	for _, e := range config {
		known[e.Path] = struct{}{}
	}
	out := append([]render.RepoEntry(nil), config...)
	for _, r := range resources {
		if r.Type != discover.ResourceRepo {
			continue
		}
		if _, ok := known[r.Path]; ok {
			continue
		}
		out = append(out, render.RepoEntry{
			Path:   r.Path,
			Type:   r.VCSType,
			Status: render.StatusUnmanaged,
		})
	}
	return out
}

// reconcileSkills merges config-driven skill entries with FS-discovered skills.
// FS-discovered skills are suppressed when their parent directory is already
// covered by a config-driven entry (same agent + same managed skills dir),
// or when the skill itself lives inside a configured skill source path —
// otherwise running gaal from inside a source repo would surface the source
// SKILL.md files as duplicate installed skills (#88). Remaining FS-discovered
// entries are appended as unmanaged.
func reconcileSkills(config []render.SkillEntry, resources []discover.Resource, home, workDir string, sourcePaths []string) []render.SkillEntry {
	// Build the set of managed skill directories from config entries.
	// Key: absolute skills directory that gaal writes to for this entry.
	managedDirs := make(map[string]struct{}, len(config))
	for _, e := range config {
		dir, ok := skill.SkillDir(e.Agent, e.Global, home)
		if !ok {
			continue
		}
		if !e.Global && !filepath.IsAbs(dir) {
			dir = filepath.Join(workDir, dir)
		}
		if e.TargetSubdir != "" {
			dir = filepath.Join(dir, e.TargetSubdir)
		}
		managedDirs[filepath.Clean(dir)] = struct{}{}
	}
	slog.Debug("reconcileSkills managed dirs", "count", len(managedDirs))

	cleanedSources := make([]string, 0, len(sourcePaths))
	for _, p := range sourcePaths {
		if p == "" {
			continue
		}
		cleanedSources = append(cleanedSources, filepath.Clean(p))
	}

	out := append([]render.SkillEntry(nil), config...)
	for _, r := range resources {
		if r.Type != discover.ResourceSkill {
			continue
		}
		// If this skill's parent directory is already managed by a config
		// entry, the skill is already accounted for — skip it to avoid
		// showing the same skill twice (once managed, once unmanaged).
		parent := filepath.Clean(filepath.Dir(r.Path))
		if _, covered := managedDirs[parent]; covered {
			continue
		}
		// Skills found inside a configured source are source content, not
		// installed copies — skip so they do not appear as duplicate rows.
		if isUnderAny(r.Path, cleanedSources) {
			continue
		}
		agent := r.Meta["agent"]
		out = append(out, render.SkillEntry{
			Source:    r.Path,
			Agent:     agent,
			Global:    r.Scope == discover.ScopeGlobal,
			Status:    render.StatusUnmanaged,
			Installed: []string{r.Name},
			Missing:   []string{},
			Modified:  []string{},
		})
	}
	return out
}

// isUnderAny reports whether path is at or below any of roots. Comparisons use
// filepath.Clean / filepath.Rel so trailing separators and relative-style
// inputs (".") are handled correctly.
func isUnderAny(path string, roots []string) bool {
	if len(roots) == 0 {
		return false
	}
	cleaned := filepath.Clean(path)
	for _, root := range roots {
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, cleaned)
		if err != nil {
			continue
		}
		if rel == "." {
			return true
		}
		if !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

// reconcileMCPs merges config-driven MCP entries with FS-discovered MCP configs.
// FS-discovered MCP config files not covered by config are appended as unmanaged.
func reconcileMCPs(config []render.MCPEntry, resources []discover.Resource) []render.MCPEntry {
	knownTargets := make(map[string]struct{}, len(config))
	for _, e := range config {
		knownTargets[e.Target] = struct{}{}
	}
	out := append([]render.MCPEntry(nil), config...)
	for _, r := range resources {
		if r.Type != discover.ResourceMCP {
			continue
		}
		if _, ok := knownTargets[r.Path]; ok {
			continue
		}
		out = append(out, render.MCPEntry{
			Name:   r.Name,
			Target: r.Path,
			Status: render.StatusUnmanaged,
		})
	}
	return out
}

// driftToStatus maps a discover.DriftState to the closest render.StatusCode.
func driftToStatus(d discover.DriftState) render.StatusCode {
	switch d {
	case discover.DriftOK:
		return render.StatusOK
	case discover.DriftModified:
		return render.StatusDirty
	case discover.DriftMissing:
		return render.StatusNotCloned
	default:
		return render.StatusOK
	}
}

func collectRepos(stats []repo.Status) []render.RepoEntry {
	entries := make([]render.RepoEntry, 0, len(stats))
	for _, st := range stats {
		e := render.RepoEntry{Path: st.Path, Type: st.Type, URL: st.URL}
		switch {
		case st.Err != nil:
			e.Status = render.StatusError
			e.Error = st.Err.Error()
		case !st.Cloned:
			e.Status = render.StatusNotCloned
		case st.Dirty:
			e.Status = render.StatusDirty
			e.Dirty = true
			e.Current = st.Current
			e.Want = orDefault(st.Version, "default")
		default:
			e.Status = render.StatusOK
			e.Current = st.Current
			e.Want = orDefault(st.Version, "default")
		}
		entries = append(entries, e)
	}
	return entries
}

func collectSkills(stats []skill.Status) []render.SkillEntry {
	entries := make([]render.SkillEntry, 0, len(stats))
	for _, st := range stats {
		e := render.SkillEntry{
			Source:       st.Source,
			Agent:        st.AgentName,
			Global:       st.Global,
			TargetSubdir: st.TargetSubdir,
			Installed:    nonNil(st.Installed),
			Missing:      nonNil(st.Missing),
			Modified:     nonNil(st.Modified),
		}
		switch {
		case st.Err != nil:
			e.Status = render.StatusError
			e.Error = st.Err.Error()
		case len(st.Missing) > 0:
			e.Status = render.StatusPartial
		case len(st.Modified) > 0:
			e.Status = render.StatusDirty
		default:
			e.Status = render.StatusOK
		}
		entries = append(entries, e)
	}
	return entries
}

func collectContent(stats []content.Status) []render.ContentEntry {
	slog.Debug("collecting content status entries", "count", len(stats))
	var entries []render.ContentEntry
	for _, st := range stats {
		if st.Err != nil {
			entries = append(entries, render.ContentEntry{
				Source: st.Source,
				Agent:  st.Agent,
				Scope:  st.Scope,
				Root:   st.Root,
				Status: render.StatusError,
				Error:  st.Err.Error(),
			})
			continue
		}
		for _, p := range st.Paths {
			e := render.ContentEntry{
				Source: st.Source,
				Agent:  st.Agent,
				Scope:  st.Scope,
				Root:   st.Root,
				Path:   p.Source,
				Target: p.Target,
			}
			switch {
			case p.Error != "":
				e.Status = render.StatusError
				e.Error = p.Error
			case !p.Present:
				e.Status = render.StatusAbsent
			case p.Dirty:
				e.Status = render.StatusDirty
			default:
				e.Status = render.StatusPresent
			}
			entries = append(entries, e)
		}
	}
	return entries
}

func collectMCPs(stats []mcp.Status) []render.MCPEntry {
	entries := make([]render.MCPEntry, 0, len(stats))
	for _, st := range stats {
		e := render.MCPEntry{Name: st.Name, Target: st.Target}
		switch {
		case st.Err != nil:
			e.Status = render.StatusError
			e.Error = st.Err.Error()
		case st.Present && st.Dirty:
			e.Status = render.StatusDirty
			e.Dirty = true
		case st.Present:
			e.Status = render.StatusPresent
		default:
			e.Status = render.StatusAbsent
		}
		entries = append(entries, e)
	}
	return entries
}

func collectAgents() ([]render.AgentEntry, error) {
	slog.Debug("collecting agent entries")
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving user home dir: %w", err)
	}
	workDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolving working dir: %w", err)
	}
	return ListAgents(home, workDir)
}

// orDefault returns s if non-empty, otherwise def.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// nonNil returns a non-nil slice (empty slice instead of nil).
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
