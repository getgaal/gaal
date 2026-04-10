package skill

import "gaal/internal/core/agent"

// AgentInfo is a type alias for agent.Info, kept for backward compatibility
// within this package.
type AgentInfo = agent.Info

// AgentNames returns all supported agent identifiers.
func AgentNames() []string { return agent.Names() }

// SkillDir returns the target skills directory for the given agent.
// If global is true the user-home directory is returned (~ expanded).
func SkillDir(agentName string, global bool, home string) (string, bool) {
	return agent.SkillDir(agentName, global, home)
}

// expandHome expands a leading ~/ or ~\ to the provided home directory.
func expandHome(p, home string) string { return agent.ExpandHome(p, home) }

// Lookup returns the Info for a registered agent name.
func Lookup(name string) (AgentInfo, bool) { return agent.Lookup(name) }

// ProjectMCPConfigPath returns the absolute path to the agent's project MCP
// configuration file (home expanded). Returns ("", false) when not supported.
func ProjectMCPConfigPath(agentName, home string) (string, bool) {
	return agent.ProjectMCPConfigPath(agentName, home)
}
