package agent

func init() {
	register("trae", Info{
		ProjectSkillsDir: ".trae/skills",
		GlobalSkillsDir:  "~/.trae/skills",
		MCPConfigFile:    "~/.trae/mcp.json",
	})
}
