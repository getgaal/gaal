package agent

func init() {
	register("amp", Info{
		ProjectSkillsDir: DefaultProjectSkillsDir,
		GlobalSkillsDir:  "~/.config/agents/skills",
		MCPConfigFile:    "~/.config/amp/settings.json",
	})
}
