package engine

import (
	"context"
	"log/slog"

	"gaal/internal/core/agent"
	"gaal/internal/mcp"
	"gaal/internal/repo"
	"gaal/internal/skill"
)

// StatusCode is the machine-readable state of a resource.
type StatusCode string

const (
	StatusOK        StatusCode = "ok"
	StatusDirty     StatusCode = "dirty"
	StatusNotCloned StatusCode = "not_cloned"
	StatusPartial   StatusCode = "partial"
	StatusPresent   StatusCode = "present"
	StatusAbsent    StatusCode = "absent"
	StatusError     StatusCode = "error"
)

// RepoEntry holds the status of a single repository.
type RepoEntry struct {
	Path    string     `json:"path"`
	Type    string     `json:"type"`
	Status  StatusCode `json:"status"`
	Dirty   bool       `json:"dirty,omitempty"`
	Current string     `json:"current,omitempty"`
	Want    string     `json:"want,omitempty"`
	URL     string     `json:"url,omitempty"`
	Error   string     `json:"error,omitempty"`
}

// SkillEntry holds the status of a single skill configuration.
type SkillEntry struct {
	Source    string     `json:"source"`
	Agent     string     `json:"agent"`
	Status    StatusCode `json:"status"`
	Installed []string   `json:"installed"`
	Missing   []string   `json:"missing"`
	Modified  []string   `json:"modified,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// AgentEntry holds the registry information for a supported agent.
type AgentEntry struct {
	Name             string `json:"name"`
	ProjectSkillsDir string `json:"project_skills_dir"`
	GlobalSkillsDir  string `json:"global_skills_dir"`
	MCPConfigFile    string `json:"mcp_config_file,omitempty"`
}

// MCPEntry holds the status of a single MCP server entry.
type MCPEntry struct {
	Name   string     `json:"name"`
	Status StatusCode `json:"status"`
	Dirty  bool       `json:"dirty,omitempty"`
	Target string     `json:"target"`
	Error  string     `json:"error,omitempty"`
}

// StatusReport aggregates the status of all managed resources.
type StatusReport struct {
	Repositories []RepoEntry  `json:"repositories"`
	Skills       []SkillEntry `json:"skills"`
	MCPs         []MCPEntry   `json:"mcps"`
	Agents       []AgentEntry `json:"agents"`
}

// Collect gathers the current status of all resources without side effects.
func (e *Engine) Collect(ctx context.Context) (*StatusReport, error) {
	slog.DebugContext(ctx, "collecting status")
	return &StatusReport{
		Repositories: collectRepos(e.repos.Status(ctx)),
		Skills:       collectSkills(e.skills.Status(ctx)),
		MCPs:         collectMCPs(e.mcps.Status(ctx)),
		Agents:       collectAgents(),
	}, nil
}

func collectRepos(stats []repo.Status) []RepoEntry {
	entries := make([]RepoEntry, 0, len(stats))
	for _, st := range stats {
		e := RepoEntry{Path: st.Path, Type: st.Type, URL: st.URL}
		switch {
		case st.Err != nil:
			e.Status = StatusError
			e.Error = st.Err.Error()
		case !st.Cloned:
			e.Status = StatusNotCloned
		case st.Dirty:
			e.Status = StatusDirty
			e.Dirty = true
			e.Current = st.Current
			e.Want = orDefault(st.Version, "default")
		default:
			e.Status = StatusOK
			e.Current = st.Current
			e.Want = orDefault(st.Version, "default")
		}
		entries = append(entries, e)
	}
	return entries
}

func collectSkills(stats []skill.Status) []SkillEntry {
	entries := make([]SkillEntry, 0, len(stats))
	for _, st := range stats {
		e := SkillEntry{
			Source:    st.Source,
			Agent:     st.AgentName,
			Installed: nonNil(st.Installed),
			Missing:   nonNil(st.Missing),
			Modified:  nonNil(st.Modified),
		}
		switch {
		case st.Err != nil:
			e.Status = StatusError
			e.Error = st.Err.Error()
		case len(st.Missing) > 0:
			e.Status = StatusPartial
		case len(st.Modified) > 0:
			e.Status = StatusDirty
		default:
			e.Status = StatusOK
		}
		entries = append(entries, e)
	}
	return entries
}

func collectMCPs(stats []mcp.Status) []MCPEntry {
	entries := make([]MCPEntry, 0, len(stats))
	for _, st := range stats {
		e := MCPEntry{Name: st.Name, Target: st.Target}
		switch {
		case st.Err != nil:
			e.Status = StatusError
			e.Error = st.Err.Error()
		case st.Present && st.Dirty:
			e.Status = StatusDirty
			e.Dirty = true
		case st.Present:
			e.Status = StatusPresent
		default:
			e.Status = StatusAbsent
		}
		entries = append(entries, e)
	}
	return entries
}

func collectAgents() []AgentEntry {
	list := agent.List()
	entries := make([]AgentEntry, len(list))
	for i, a := range list {
		entries[i] = AgentEntry{
			Name:             a.Name,
			ProjectSkillsDir: a.Info.ProjectSkillsDir,
			GlobalSkillsDir:  a.Info.GlobalSkillsDir,
			MCPConfigFile:    a.Info.MCPConfigFile,
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
