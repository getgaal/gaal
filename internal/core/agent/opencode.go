package agent

func init() {
	register("opencode", Info{
		ProjectSkillsDir: DefaultProjectSkillsDir,
		GlobalSkillsDir:  "~/.config/opencode/skills",
		MCPConfigFile:    "~/.config/opencode/config.json",
	})
}
