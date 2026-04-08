package agent

func init() {
	register("continue", Info{
		ProjectSkillsDir: ".continue/skills",
		GlobalSkillsDir:  "~/.continue/skills",
		MCPConfigFile:    "~/.continue/config.json",
	})
}
