package skill

import (
	"path/filepath"
	"strings"
)

// AgentInfo holds the install path details for a coding agent.
type AgentInfo struct {
	// ProjectDir is the skills directory relative to the project root.
	ProjectDir string
	// GlobalDir is the skills directory in the user's home directory.
	GlobalDir string
}

// knownAgents is the registry of all supported coding agents and their skill
// directories (project-scoped, then global). Global paths use the ~ prefix
// which is expanded at runtime.
var knownAgents = map[string]AgentInfo{
	"amp":            {".agents/skills", "~/.config/agents/skills"},
	"antigravity":    {".agents/skills", "~/.gemini/antigravity/skills"},
	"augment":        {".augment/skills", "~/.augment/skills"},
	"claude-code":    {".claude/skills", "~/.claude/skills"},
	"cline":          {".agents/skills", "~/.agents/skills"},
	"codex":          {".agents/skills", "~/.codex/skills"},
	"continue":       {".continue/skills", "~/.continue/skills"},
	"cursor":         {".agents/skills", "~/.cursor/skills"},
	"gemini-cli":     {".agents/skills", "~/.gemini/skills"},
	"github-copilot": {".agents/skills", "~/.copilot/skills"},
	"goose":          {".goose/skills", "~/.config/goose/skills"},
	"kilo":           {".kilocode/skills", "~/.kilocode/skills"},
	"kiro-cli":       {".kiro/skills", "~/.kiro/skills"},
	"opencode":       {".agents/skills", "~/.config/opencode/skills"},
	"openhands":      {".openhands/skills", "~/.openhands/skills"},
	"roo":            {".roo/skills", "~/.roo/skills"},
	"trae":           {".trae/skills", "~/.trae/skills"},
	"windsurf":       {".windsurf/skills", "~/.codeium/windsurf/skills"},
	"warp":           {".agents/skills", "~/.agents/skills"},
	"zencoder":       {".zencoder/skills", "~/.zencoder/skills"},
}

// AgentNames returns all supported agent identifiers.
func AgentNames() []string {
	names := make([]string, 0, len(knownAgents))
	for k := range knownAgents {
		names = append(names, k)
	}
	return names
}

// SkillDir returns the target skills directory for the given agent.
// If global is true the user-home directory is returned.
func SkillDir(agentName string, global bool, home string) (string, bool) {
	info, ok := knownAgents[agentName]
	if !ok {
		return "", false
	}
	if global {
		dir := expandHome(info.GlobalDir, home)
		return dir, true
	}
	return info.ProjectDir, true
}

func expandHome(p, home string) string {
	// Accept both ~/ (POSIX) and ~\ (Windows) as home-relative prefixes.
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}
