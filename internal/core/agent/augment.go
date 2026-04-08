package agent

func init() {
	register("augment", Info{
		ProjectSkillsDir: ".augment/skills",
		GlobalSkillsDir:  "~/.augment/skills",
		MCPConfigFile:    "~/.augment/settings.json",
	})
}
