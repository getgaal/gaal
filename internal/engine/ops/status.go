package ops

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"gaal/internal/discover"
	"gaal/internal/engine/render"
	"gaal/internal/mcp"
	"gaal/internal/repo"
	"gaal/internal/skill"
)

// Collect gathers the current status of all resources without side effects.
// It performs a FS-first scan via discover.Scan and then reconciles with any
// config-declared resources from the managers, marking them as managed.
func Collect(ctx context.Context, repos *repo.Manager, skills *skill.Manager, mcps *mcp.Manager, home, workDir, stateDir string) (*render.StatusReport, error) {
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
	configMCPs := collectMCPs(mcps.Status(ctx))

	// Reconcile: mark config-declared resources as managed and merge
	// FS-discovered unmanaged resources into the report.
	repoEntries := reconcileRepos(configRepos, discovered)
	skillEntries := reconcileSkills(configSkills, discovered)
	mcpEntries := reconcileMCPs(configMCPs, discovered)

	return &render.StatusReport{
		Repositories: repoEntries,
		Skills:       skillEntries,
		MCPs:         mcpEntries,
		Agents:       agents,
	}, nil
}

// Status collects the current resource state and renders it to os.Stdout.
func Status(ctx context.Context, repos *repo.Manager, skills *skill.Manager, mcps *mcp.Manager, home, workDir, stateDir string, format render.OutputFormat) error {
	slog.DebugContext(ctx, "status requested", "format", format)

	report, err := Collect(ctx, repos, skills, mcps, home, workDir, stateDir)
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
// FS-discovered skills not covered by config are appended as unmanaged entries.
func reconcileSkills(config []render.SkillEntry, resources []discover.Resource) []render.SkillEntry {
	known := make(map[string]struct{}, len(config))
	for _, e := range config {
		known[e.Source+"/"+e.Agent] = struct{}{}
	}
	out := append([]render.SkillEntry(nil), config...)
	for _, r := range resources {
		if r.Type != discover.ResourceSkill {
			continue
		}
		agent := r.Meta["agent"]
		key := r.Path + "/" + agent
		if _, ok := known[key]; ok {
			continue
		}
		out = append(out, render.SkillEntry{
			Source:    r.Path,
			Agent:     agent,
			Status:    render.StatusUnmanaged,
			Installed: []string{r.Name},
			Missing:   []string{},
			Modified:  []string{},
		})
	}
	return out
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
			Source:    st.Source,
			Agent:     st.AgentName,
			Installed: nonNil(st.Installed),
			Missing:   nonNil(st.Missing),
			Modified:  nonNil(st.Modified),
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
