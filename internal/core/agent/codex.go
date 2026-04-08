package agent

func init() {
	register("codex", Info{
		ProjectSkillsDir: DefaultProjectSkillsDir,
		GlobalSkillsDir:  "~/.codex/skills",
		MCPConfigFile:    "~/.codex/mcp.json",
	})
}
