package agent

func init() {
	register("kilo", Info{
		ProjectSkillsDir: ".kilocode/skills",
		GlobalSkillsDir:  "~/.kilocode/skills",
		MCPConfigFile:    "~/.kilocode/mcp.json",
	})
}
