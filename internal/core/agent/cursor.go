package agent

func init() {
	register("cursor", Info{
		ProjectSkillsDir: DefaultProjectSkillsDir,
		GlobalSkillsDir:  "~/.cursor/skills",
		MCPConfigFile:    "~/.cursor/mcp.json",
	})
}
