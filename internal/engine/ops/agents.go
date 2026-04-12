package ops

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"gaal/internal/core/agent"
	"gaal/internal/engine/render"
	"gaal/internal/skill"
)

// ListAgents returns all registered agents with installed-detection,
// sorted installed-first then alphabetically.
func ListAgents(home, workDir string) ([]render.AgentEntry, error) {
	slog.Debug("listing agents", "home", home, "workDir", workDir)

	list := agent.List()
	builtinSet := make(map[string]struct{}, len(agent.Names()))
	for _, n := range agent.Names() {
		builtinSet[n] = struct{}{}
	}

	entries := make([]render.AgentEntry, 0, len(list))
	for _, a := range list {
		projectDir, ok := agent.SkillDir(a.Name, false, home)
		if !ok {
			return nil, fmt.Errorf("resolving project skills dir for agent %q", a.Name)
		}
		globalDir, ok := agent.SkillDir(a.Name, true, home)
		if !ok {
			return nil, fmt.Errorf("resolving global skills dir for agent %q", a.Name)
		}

		installed := skill.IsAgentInstalled(a.Name, false, home, workDir) ||
			skill.IsAgentInstalled(a.Name, true, home, workDir)

		source := "builtin"
		if _, ok := builtinSet[a.Name]; !ok {
			source = "user"
		}

		entries = append(entries, render.AgentEntry{
			Name:                    a.Name,
			Installed:               installed,
			Source:                  source,
			ProjectSkillsDir:        projectDir,
			GlobalSkillsDir:         globalDir,
			ProjectMCPConfigFile:    a.Info.ProjectMCPConfigFile,
			ProjectSkillsViaGeneric: a.Info.SupportsGenericProject,
			GlobalSkillsViaGeneric:  a.Info.SupportsGenericGlobal,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Installed != entries[j].Installed {
			return entries[i].Installed
		}
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// AgentDetail returns the full detail view for a single agent identified
// by name (case-insensitive match).
func AgentDetail(home, workDir, name string) (*render.AgentDetail, error) {
	slog.Debug("agent detail", "name", name)

	var match *agent.Entry
	for _, a := range agent.List() {
		if strings.EqualFold(a.Name, name) {
			a := a
			match = &a
			break
		}
	}
	if match == nil {
		return nil, fmt.Errorf("unknown agent %q", name)
	}

	builtinSet := make(map[string]struct{}, len(agent.Names()))
	for _, n := range agent.Names() {
		builtinSet[n] = struct{}{}
	}

	installed := skill.IsAgentInstalled(match.Name, false, home, workDir) ||
		skill.IsAgentInstalled(match.Name, true, home, workDir)

	source := "builtin"
	if _, ok := builtinSet[match.Name]; !ok {
		source = "user"
	}

	paths := collectAgentPaths(match.Name, home, workDir)

	mcpCfg, mcpOk := agent.ProjectMCPConfigPath(match.Name, home)
	var mcpExists bool
	if mcpOk {
		_, err := os.Stat(mcpCfg)
		mcpExists = err == nil
	}

	return &render.AgentDetail{
		Name:       match.Name,
		Installed:  installed,
		Source:     source,
		Paths:      paths,
		MCPSupport: match.Info.ProjectMCPConfigFile != "",
		MCPConfig:  mcpCfg,
		MCPExists:  mcpExists,
	}, nil
}

// collectAgentPaths gathers all search paths for an agent with existence
// and skill-count metadata.
func collectAgentPaths(name, home, workDir string) []render.AgentPath {
	var paths []render.AgentPath

	addPath := func(label, dir string) {
		exists := false
		count := 0
		if _, err := os.Stat(dir); err == nil {
			exists = true
			if metas, err := skill.ScanDir(dir); err == nil {
				count = len(metas)
			}
		}
		paths = append(paths, render.AgentPath{
			Label:      label,
			Path:       dir,
			Exists:     exists,
			SkillCount: count,
		})
	}

	for _, relDir := range agent.ExpandedProjectSkillsSearch(name) {
		absDir := workDir + "/" + relDir
		addPath("project", absDir)
	}

	for _, absDir := range agent.ExpandedGlobalSkillsSearch(name, home) {
		addPath("global", absDir)
	}

	for _, absDir := range agent.ExpandedPmSkillsSearch(name, home) {
		addPath("package-manager", absDir)
	}

	return paths
}
