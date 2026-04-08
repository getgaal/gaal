package agent

func init() {
	register("claude-code", Info{
		ProjectSkillsDir: ".claude/skills",
		GlobalSkillsDir:  "~/.claude/skills",
		MCPConfigFile:    "~/.config/claude/claude_desktop_config.json",
	})
}
