package agent

import (
	"path/filepath"
	"strings"
)

// DefaultProjectSkillsDir is the project-scoped skills directory used by agents
// that follow the shared .agents/skills convention.
const DefaultProjectSkillsDir = ".agents/skills"

// Info describes the file-system layout for a coding agent: where skills are
// installed and where the MCP server configuration file lives.
type Info struct {
	// ProjectSkillsDir is the skills directory relative to the project root.
	ProjectSkillsDir string
	// GlobalSkillsDir is the skills directory under the user home directory (~).
	GlobalSkillsDir string
	// MCPConfigFile is the path to the agent's MCP server configuration file,
	// relative to the user home directory (~). Empty when unknown or unsupported.
	MCPConfigFile string
}

// registry holds all registered agents, populated by each agent file's init().
var registry = map[string]Info{}

// register adds an agent to the registry. Panics on duplicate names so
// conflicts are caught at startup.
func register(name string, info Info) {
	if _, exists := registry[name]; exists {
		panic("agent: duplicate registration: " + name)
	}
	registry[name] = info
}

// Names returns all supported agent identifiers.
func Names() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}

// Lookup returns the Info for name and whether it was found.
func Lookup(name string) (Info, bool) {
	info, ok := registry[name]
	return info, ok
}

// SkillDir returns the target skills directory for the given agent.
// If global is true the user-home directory is returned (~ expanded).
func SkillDir(name string, global bool, home string) (string, bool) {
	info, ok := registry[name]
	if !ok {
		return "", false
	}
	if global {
		return ExpandHome(info.GlobalSkillsDir, home), true
	}
	return info.ProjectSkillsDir, true
}

// MCPConfigPath returns the absolute path to the agent's MCP configuration
// file (home expanded). Returns ("", false) when not known for this agent.
func MCPConfigPath(name, home string) (string, bool) {
	info, ok := registry[name]
	if !ok || info.MCPConfigFile == "" {
		return "", false
	}
	return ExpandHome(info.MCPConfigFile, home), true
}

// ExpandHome expands a leading ~/ or ~\ to the provided home directory.
func ExpandHome(p, home string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}
