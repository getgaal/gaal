package agent

func init() {
	register("kiro-cli", Info{
		ProjectSkillsDir: ".kiro/skills",
		GlobalSkillsDir:  "~/.kiro/skills",
		MCPConfigFile:    "~/.kiro/settings/mcp.json",
	})
}
