package agent

func init() {
	register("zencoder", Info{
		ProjectSkillsDir: ".zencoder/skills",
		GlobalSkillsDir:  "~/.zencoder/skills",
		MCPConfigFile:    "~/.zencoder/mcp.json",
	})
}
