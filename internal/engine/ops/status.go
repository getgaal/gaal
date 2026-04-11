package ops

import (
	"context"
	"log/slog"
	"os"

	"gaal/internal/core/agent"
	"gaal/internal/engine/render"
	"gaal/internal/mcp"
	"gaal/internal/repo"
	"gaal/internal/skill"
)

// Collect gathers the current status of all resources without side effects.
func Collect(ctx context.Context, repos *repo.Manager, skills *skill.Manager, mcps *mcp.Manager) (*render.StatusReport, error) {
	slog.DebugContext(ctx, "collecting status")
	return &render.StatusReport{
		Repositories: collectRepos(repos.Status(ctx)),
		Skills:       collectSkills(skills.Status(ctx)),
		MCPs:         collectMCPs(mcps.Status(ctx)),
		Agents:       collectAgents(),
	}, nil
}

// Status collects the current resource state and renders it to os.Stdout
// using the specified format (FormatTable or FormatJSON).
func Status(ctx context.Context, repos *repo.Manager, skills *skill.Manager, mcps *mcp.Manager, format render.OutputFormat) error {
	slog.DebugContext(ctx, "status requested", "format", format)

	report, err := Collect(ctx, repos, skills, mcps)
	if err != nil {
		return err
	}

	renderer, err := render.NewRenderer(format)
	if err != nil {
		return err
	}

	return renderer.Render(os.Stdout, report)
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

func collectAgents() []render.AgentEntry {
	list := agent.List()
	entries := make([]render.AgentEntry, len(list))
	for i, a := range list {
		entries[i] = render.AgentEntry{
			Name:                 a.Name,
			ProjectSkillsDir:     a.Info.ProjectSkillsDir,
			GlobalSkillsDir:      a.Info.GlobalSkillsDir,
			ProjectMCPConfigFile: a.Info.ProjectMCPConfigFile,
		}
	}
	return entries
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
