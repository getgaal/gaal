package ops

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"gaal/internal/config"
	"gaal/internal/core/agent"
	"gaal/internal/engine/render"
	"gaal/internal/mcp"
)

// BuildImportCandidates runs the same discovery passes as Audit, filters them
// by scope, resolves best-effort source strings for skills, and loads the full
// inline definition of every detected MCP server.
//
// The returned Candidates list is grouped by agent and sorted by agent name.
// Sections with no skill AND no MCP are omitted, so the caller only ever sees
// agents that actually have importable content.
func BuildImportCandidates(ctx context.Context, scope Scope, home, workDir, cacheRoot string) (Candidates, error) {
	slog.DebugContext(ctx, "building import candidates",
		"scope", scope, "home", home, "workDir", workDir, "cacheRoot", cacheRoot)

	skills, err := collectAuditSkills(ctx, home, workDir)
	if err != nil {
		return Candidates{}, fmt.Errorf("collecting audit skills: %w", err)
	}

	filteredSkills := filterSkillsByScope(skills, scope)

	mcps, err := collectInitMCPs(ctx, home)
	if err != nil {
		return Candidates{}, fmt.Errorf("collecting mcps: %w", err)
	}

	byAgent := map[string]*AgentSection{}

	// Dedupe skills by (agent, skillName), keeping the candidate with the
	// best-ranked source (git > cache > path). When the same skill shows up
	// via several search paths — which is common when an agent overlays a
	// package-manager plugin over a user-installed skill — the user only
	// sees it once.
	type skillKey struct{ agent, name string }
	bestSkill := map[skillKey]Candidate{}

	for _, s := range filteredSkills {
		src, kind := ResolveSkillSource(s.Path, cacheRoot)
		cand := Candidate{
			Kind:            CandidateSkill,
			AgentName:       s.Agent,
			Label:           fmt.Sprintf("skill: %s", s.Name),
			Detail:          detailForSkill(src, s.Path),
			SkillName:       s.Name,
			SkillDesc:       s.Desc,
			SkillPath:       s.Path,
			SkillSource:     src,
			SkillSourceKind: kind,
		}
		key := skillKey{agent: s.Agent, name: s.Name}
		if existing, ok := bestSkill[key]; !ok || sourceKindRank(kind) > sourceKindRank(existing.SkillSourceKind) {
			bestSkill[key] = cand
		}
	}

	for _, cand := range bestSkill {
		sec := byAgent[cand.AgentName]
		if sec == nil {
			sec = &AgentSection{AgentName: cand.AgentName}
			byAgent[cand.AgentName] = sec
		}
		sec.Skills = append(sec.Skills, cand)
	}

	for _, m := range mcps {
		sec := byAgent[m.agent]
		if sec == nil {
			sec = &AgentSection{AgentName: m.agent}
			byAgent[m.agent] = sec
		}
		for name, inline := range m.servers {
			inlineCopy := inline
			sec.MCPs = append(sec.MCPs, Candidate{
				Kind:      CandidateMCP,
				AgentName: m.agent,
				Label:     fmt.Sprintf("mcp:   %s", name),
				Detail:    m.file,
				MCPName:   name,
				MCPTarget: m.file,
				MCPInline: &inlineCopy,
			})
		}
	}

	for _, sec := range byAgent {
		sort.Slice(sec.Skills, func(i, j int) bool { return sec.Skills[i].SkillName < sec.Skills[j].SkillName })
		sort.Slice(sec.MCPs, func(i, j int) bool { return sec.MCPs[i].MCPName < sec.MCPs[j].MCPName })
	}

	if sec := byAgent["generic"]; sec != nil {
		sec.GenericDelegators = genericDelegators(scope)
	}

	names := make([]string, 0, len(byAgent))
	for name := range byAgent {
		names = append(names, name)
	}
	sort.Strings(names)
	sections := make([]AgentSection, 0, len(names))
	for _, name := range names {
		sections = append(sections, *byAgent[name])
	}
	return Candidates{Sections: sections}, nil
}

// filterSkillsByScope keeps only audit skills whose Source matches the chosen
// gaal scope:
//
//   - ScopeProject  → "project"
//   - ScopeGlobal   → "global" and "package-manager"
func filterSkillsByScope(in []render.AuditSkillEntry, scope Scope) []render.AuditSkillEntry {
	out := make([]render.AuditSkillEntry, 0, len(in))
	for _, s := range in {
		switch scope {
		case ScopeProject:
			if s.Source == "project" {
				out = append(out, s)
			}
		case ScopeGlobal:
			if s.Source == "global" || s.Source == "package-manager" {
				out = append(out, s)
			}
		}
	}
	return out
}

// initMCPEntry is a local shape holding the full inline definition loaded
// from one agent's MCP config file.
type initMCPEntry struct {
	agent   string
	file    string
	servers map[string]config.ConfigMcpItem
}

// collectInitMCPs walks every registered agent, resolves its MCP config path,
// and loads the full set of inline definitions. Agents without a config path
// or whose file cannot be parsed are skipped (warn-level log). Servers
// without a command (e.g. URL-based HTTP transports) cannot be round-tripped
// through gaal.yaml today and are dropped with a debug log.
func collectInitMCPs(ctx context.Context, home string) ([]initMCPEntry, error) {
	slog.DebugContext(ctx, "collecting init mcps")
	var entries []initMCPEntry

	for _, a := range agent.List() {
		cfgFile, ok := agent.ProjectMCPConfigPath(a.Name, home)
		if !ok {
			continue
		}
		servers, err := mcp.LoadServers(cfgFile)
		if err != nil {
			slog.WarnContext(ctx, "cannot load mcp config file, skipping agent",
				"agent", a.Name, "file", cfgFile, "err", err)
			continue
		}
		supported := map[string]config.ConfigMcpItem{}
		for name, inline := range servers {
			if inline.Command == "" {
				slog.DebugContext(ctx, "skipping mcp server without command",
					"agent", a.Name, "server", name, "file", cfgFile)
				continue
			}
			supported[name] = inline
		}
		if len(supported) == 0 {
			continue
		}
		entries = append(entries, initMCPEntry{
			agent:   a.Name,
			file:    cfgFile,
			servers: supported,
		})
	}
	return entries, nil
}

// genericDelegators returns the sorted list of agent names that natively
// read from the generic project or global convention for the given scope.
func genericDelegators(scope Scope) []string {
	var out []string
	for _, a := range agent.List() {
		switch scope {
		case ScopeProject:
			if a.Info.SupportsGenericProject && a.Name != "generic" {
				out = append(out, a.Name)
			}
		case ScopeGlobal:
			if a.Info.SupportsGenericGlobal && a.Name != "generic" {
				out = append(out, a.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// detailForSkill chooses the grey trailing string rendered by the wizard for
// a skill candidate: we prefer the resolved source when it differs from the
// raw disk path, otherwise we show the path itself.
func detailForSkill(source, path string) string {
	if source == path {
		return path
	}
	return source
}

// sourceKindRank ranks source resolution outcomes so Dedupe can keep the
// most informative one per skill. Higher is better.
func sourceKindRank(k SourceKind) int {
	switch k {
	case SourceKindGit:
		return 3
	case SourceKindCache:
		return 2
	case SourceKindPath:
		return 1
	default:
		return 0
	}
}
