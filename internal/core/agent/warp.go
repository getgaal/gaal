package agent

func init() {
	register("warp", Info{
		ProjectSkillsDir: DefaultProjectSkillsDir,
		GlobalSkillsDir:  "~/.agents/skills",
		MCPConfigFile:    "~/.warp/launch_configurations/mcp.json",
	})
}
