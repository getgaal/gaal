package agent

func init() {
	register("windsurf", Info{
		ProjectSkillsDir: ".windsurf/skills",
		GlobalSkillsDir:  "~/.codeium/windsurf/skills",
		MCPConfigFile:    "~/.codeium/windsurf/mcp_settings.json",
	})
}
